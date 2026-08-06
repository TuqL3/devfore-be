// Package openrouter asks a model for one JSON object and returns it. Nothing in
// here knows what a scenario is: the caller supplies the system prompt, the JSON
// schema and the conversation, and gets bytes back.
//
// The split matters because the thing that makes this safe is not the model —
// it is `sim.CheckScenario` and the engine running the author's own examples
// afterwards. Keeping the transport ignorant of the domain is what stops that
// check from drifting into "the model said it was fine".
//
// Written against net/http rather than an SDK. The request is one JSON POST and
// the response is one JSON body; an SDK for that is a dependency, a version to
// track and an upgrade to do, in exchange for about sixty lines.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devforge/be/internal/labs/domain"
)

// ErrDisabled is returned when no API key is configured. The server still
// starts: this is one optional feature, not a boot requirement, and a deployment
// without a key should refuse this one endpoint rather than fail to come up.
var ErrDisabled = errors.New("chưa cấu hình OPENROUTER_API_KEY")

// ErrRefused is the model declining rather than answering. It arrives as a
// successful response whose content is empty, not as an HTTP error, so a caller
// that reads content without checking gets an unexplained blank.
var ErrRefused = errors.New("model từ chối yêu cầu này")

const (
	endpoint = "https://openrouter.ai/api/v1/chat/completions"
	// The request is not streamed, so the whole answer has to arrive inside one
	// timeout. Generous because a schema-constrained answer of this size takes a
	// while on a reasoning model, and finite because a hung request otherwise
	// holds a goroutine and a client connection until the process restarts.
	timeout = 3 * time.Minute
	// Room for a catalogue of the size the engine accepts plus its examples.
	maxTokens = 16000
)

type Client struct {
	key   string
	model string
	// Sent as HTTP-Referer and X-OpenRouter-Title: OpenRouter uses them to
	// attribute traffic to an app. Not authentication and not required.
	appURL  string
	appName string
	http    *http.Client
}

// New returns a disabled client when the key or model is empty rather than an
// error, so the caller can wire it unconditionally and let the endpoint answer
// for itself.
func New(apiKey, model, appURL string) *Client {
	return &Client{
		key:     apiKey,
		model:   model,
		appURL:  appURL,
		appName: "DevForge",
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool { return c.key != "" && c.model != "" }

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	// Constrains generation to the schema, so what comes back validates against
	// it or the request fails. That removes the whole retry-on-parse layer this
	// would otherwise need.
	ResponseFormat responseFormat `json:"response_format"`
	Provider       provider       `json:"provider"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

// provider routes the request. RequireParameters keeps it away from any backend
// that does not honour response_format — without it the request still succeeds
// and comes back as prose, which is the worst outcome available here: a caller
// that asked for a schema gets something that only looks like an answer.
type provider struct {
	RequireParameters bool `json:"require_parameters"`
}

type response struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// JSON asks for one object matching schema and returns it verbatim.
func (c *Client) JSON(
	ctx context.Context, system string, turns []domain.AITurn, schema map[string]any,
) (json.RawMessage, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}

	msgs := make([]message, 0, len(turns)+1)
	msgs = append(msgs, message{Role: "system", Content: system})
	for _, t := range turns {
		role := "user"
		if t.Assistant {
			role = "assistant"
		}
		msgs = append(msgs, message{Role: role, Content: t.Text})
	}

	body, err := json.Marshal(request{
		Model:     c.model,
		Messages:  msgs,
		MaxTokens: maxTokens,
		ResponseFormat: responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchema{
				Name:   "sim_scenario",
				Strict: true,
				Schema: schema,
			},
		},
		Provider: provider{RequireParameters: true},
	})
	if err != nil {
		return nil, fmt.Errorf("dựng request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dựng request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	if c.appURL != "" {
		req.Header.Set("HTTP-Referer", c.appURL)
		req.Header.Set("X-OpenRouter-Title", c.appName)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gọi model: %w", err)
	}
	defer res.Body.Close()

	// Bounded read. The body is one JSON object of a size this request already
	// capped; anything far past that is a proxy or an error page, and streaming
	// it into memory helps nobody.
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("đọc kết quả: %w", err)
	}

	var out response
	// Decoded before the status check: OpenRouter puts the useful sentence in the
	// body, and "HTTP 400" on its own tells a maintainer nothing.
	_ = json.Unmarshal(raw, &out)
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("model báo lỗi: %s", out.Error.Message)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model trả về HTTP %d: %s", res.StatusCode, snippet(raw))
	}
	if len(out.Choices) == 0 {
		return nil, ErrRefused
	}

	choice := out.Choices[0]
	// A truncated answer is not valid JSON, and reporting it as a parse failure
	// would send the repair loop chasing a schema problem that does not exist.
	if choice.FinishReason == "length" {
		return nil, errors.New("kết quả bị cắt giữa chừng vì quá dài")
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return nil, ErrRefused
	}
	return json.RawMessage(choice.Message.Content), nil
}

// snippet keeps an unexpected body short enough to log. An HTML error page pasted
// whole into a server log buries everything around it.
func snippet(b []byte) string {
	const max = 300
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
