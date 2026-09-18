package simgen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/devforge/be/internal/labs/domain"
)

// fakeGen replays canned answers in order, so a test can say "the model gets it
// wrong first, right second" without a network call or an API key.
type fakeGen struct {
	answers  []string
	calls    int
	turns    [][]Turn
	disabled bool
}

func (f *fakeGen) Enabled() bool { return !f.disabled }

func (f *fakeGen) JSON(
	_ context.Context, _ string, turns []domain.AITurn, _ map[string]any,
) (json.RawMessage, error) {
	f.turns = append(f.turns, append([]Turn{}, turns...))
	if f.calls >= len(f.answers) {
		return nil, errors.New("gọi nhiều hơn số câu trả lời đã chuẩn bị")
	}
	out := f.answers[f.calls]
	f.calls++
	return json.RawMessage(out), nil
}

type openQuota struct{ taken int }

func (q *openQuota) Take(context.Context, int64) error { q.taken++; return nil }

type closedQuota struct{}

func (closedQuota) Take(context.Context, int64) error { return domain.ErrAIQuota }

// A scenario that runs: two steps, one artifact chain, one parsable example.
const goodJSON = `{
  "notes": "bộ step Node tối giản",
  "runner_count": 2,
  "cache_restore_seconds": 10,
  "steps": [
    {"name":"checkout","seconds":5,"cacheable":"","produces":"","consumes":"","flaky":0},
    {"name":"npm-build","seconds":60,"cacheable":"","produces":"dist","consumes":"","flaky":0}
  ],
  "examples": [
    {"title":"① Nối tiếp","note":"một job làm hết","pipeline":"jobs:\n  ci:\n    steps: [checkout, npm-build]\n"}
  ]
}`

func run(t *testing.T, answers ...string) (*Result, *fakeGen, error) {
	t.Helper()
	gen := &fakeGen{answers: answers}
	s := New(gen, &openQuota{})
	res, err := s.Generate(context.Background(), 1, Input{Prompt: "dựng cho tôi pipeline node"})
	return res, gen, err
}

func TestGenerateAcceptsARunnableScenario(t *testing.T) {
	res, gen, err := run(t, goodJSON)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("want 1 model call, got %d", gen.calls)
	}
	if res.Attempts != 1 {
		t.Fatalf("want attempts=1, got %d", res.Attempts)
	}
	if _, ok := res.Scenario.Catalog["npm-build"]; !ok {
		t.Fatalf("steps did not become a catalogue: %#v", res.Scenario.Catalog)
	}
	if res.Scenario.Catalog["npm-build"].Produces != "dist" {
		t.Fatalf("step fields lost in conversion: %#v", res.Scenario.Catalog["npm-build"])
	}
}

// The whole reason this package exists: a scenario the engine refuses must not
// reach the client, and the engine's own sentence must be what gets sent back
// for the repair.
func TestGenerateRepairsFromTheEnginesOwnError(t *testing.T) {
	// runner_count of 99 is over the ceiling CheckScenario enforces.
	broken := strings.Replace(goodJSON, `"runner_count": 2`, `"runner_count": 99`, 1)

	res, gen, err := run(t, broken, goodJSON)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gen.calls != 2 {
		t.Fatalf("want 2 model calls, got %d", gen.calls)
	}
	if res.Attempts != 2 {
		t.Fatalf("want attempts=2, got %d", res.Attempts)
	}

	second := gen.turns[1]
	last := second[len(second)-1].Text
	if !strings.Contains(last, "runner_count là 99") {
		t.Fatalf("repair turn did not carry the engine's error:\n%s", last)
	}
	if !second[len(second)-2].Assistant {
		t.Fatalf("the rejected JSON was not echoed back as an assistant turn")
	}
}

// An example that does not parse is a button that breaks on click — it has to
// be caught here, not by the student.
func TestGenerateRejectsAnExampleThatDoesNotParse(t *testing.T) {
	bad := strings.Replace(goodJSON,
		`steps: [checkout, npm-build]`, `steps: [checkout, deploy-to-mars]`, 1)

	_, gen, err := run(t, bad, bad)
	if !errors.Is(err, ErrUnusable) {
		t.Fatalf("want ErrUnusable, got %v", err)
	}
	if gen.calls != 2 {
		t.Fatalf("want the repair round to be tried once, got %d calls", gen.calls)
	}
	if !strings.Contains(err.Error(), "deploy-to-mars") {
		t.Fatalf("error should name the step the example invented: %v", err)
	}
}

// A scenario with no examples is technically valid to the engine and useless to
// a newcomer: the lesson is a comparison, and there is nothing to compare.
func TestGenerateRejectsAScenarioWithNoExamples(t *testing.T) {
	none := strings.Replace(goodJSON,
		`"examples": [
    {"title":"① Nối tiếp","note":"một job làm hết","pipeline":"jobs:\n  ci:\n    steps: [checkout, npm-build]\n"}
  ]`, `"examples": []`, 1)

	if _, _, err := run(t, none, none); !errors.Is(err, ErrUnusable) {
		t.Fatalf("want ErrUnusable, got %v", err)
	}
}

func TestGenerateRejectsDuplicateStepNames(t *testing.T) {
	dup := strings.Replace(goodJSON,
		`{"name":"npm-build","seconds":60`, `{"name":"checkout","seconds":60`, 1)

	_, _, err := run(t, dup, dup)
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("want a duplicate-name rejection, got %v", err)
	}
}

// The quota is a cost ceiling, so it has to be spent before the model is called
// — not after, where a refusal would still have been billed.
func TestQuotaIsCheckedBeforeTheModelIsCalled(t *testing.T) {
	gen := &fakeGen{answers: []string{goodJSON}}
	s := New(gen, closedQuota{})

	_, err := s.Generate(context.Background(), 1, Input{Prompt: "bất kỳ"})
	if !errors.Is(err, domain.ErrAIQuota) {
		t.Fatalf("want ErrAIQuota, got %v", err)
	}
	if gen.calls != 0 {
		t.Fatalf("quota was spent but the model was called anyway (%d times)", gen.calls)
	}
}

// A deployment with no API key must turn the request away before charging for
// it. Otherwise every 503 eats a slot, and the day the key is added the user has
// none left — a cost control that costs the user something is a bug.
func TestDisabledGeneratorCostsNoQuota(t *testing.T) {
	gen := &fakeGen{answers: []string{goodJSON}, disabled: true}
	q := &openQuota{}

	_, err := New(gen, q).Generate(context.Background(), 1, Input{Prompt: "bất kỳ"})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
	if q.taken != 0 {
		t.Fatalf("a switched-off feature spent %d quota", q.taken)
	}
}

// An empty or oversized prompt costs nothing to reject, so it must not consume a
// quota slot either.
func TestBadPromptsCostNothing(t *testing.T) {
	for name, prompt := range map[string]string{
		"empty":    "   ",
		"too long": strings.Repeat("a", MaxPromptChars+1),
	} {
		t.Run(name, func(t *testing.T) {
			gen := &fakeGen{answers: []string{goodJSON}}
			q := &openQuota{}
			if _, err := New(gen, q).Generate(context.Background(), 1, Input{Prompt: prompt}); err == nil {
				t.Fatal("want a rejection")
			}
			if gen.calls != 0 || q.taken != 0 {
				t.Fatalf("a rejected prompt spent something: calls=%d quota=%d", gen.calls, q.taken)
			}
		})
	}
}

// A long conversation must not resend an unbounded history — that is billed per
// token on every turn.
func TestHistoryIsTrimmedToTheTail(t *testing.T) {
	history := make([]Turn, 40)
	for i := range history {
		history[i] = Turn{Text: "turn"}
	}
	history[len(history)-1] = Turn{Text: "cái cuối cùng"}

	gen := &fakeGen{answers: []string{goodJSON}}
	if _, err := New(gen, &openQuota{}).Generate(context.Background(), 1, Input{
		Prompt: "sửa lại", History: history,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	sent := gen.turns[0]
	if len(sent) != MaxTurns+1 {
		t.Fatalf("want %d trimmed turns plus the new prompt, got %d", MaxTurns, len(sent))
	}
	if sent[len(sent)-2].Text != "cái cuối cùng" {
		t.Fatalf("trimming kept the head instead of the tail: %q", sent[len(sent)-2].Text)
	}
}

// The worked example in the system prompt is the strongest signal the model
// gets. If it stops running, the prompt is teaching the model to produce
// something the engine rejects — and that failure is invisible until somebody
// reads generated output closely. So it goes through the same `decode` +
// `verify` path the model's own answer takes.
func TestWorkedExampleIsRunnable(t *testing.T) {
	sc, notes, err := decode([]byte(workedExample))
	if err != nil {
		t.Fatalf("ví dụ mẫu trong prompt không decode được: %v", err)
	}
	if err := verify(sc); err != nil {
		t.Fatalf("ví dụ mẫu trong prompt không chạy được: %v", err)
	}
	if notes == "" {
		t.Fatal("ví dụ mẫu không có notes — model sẽ học theo và bỏ trống trường đó")
	}
	// It has to demonstrate the comparison it is teaching, not just parse: the
	// scenario needs an expensive step and a cacheable one, or every arrangement
	// of it costs the same and there is no lesson to copy.
	var expensive, cacheable int
	for _, st := range sc.Catalog {
		if st.Seconds >= 60 {
			expensive++
		}
		if st.Cacheable != "" {
			cacheable++
		}
	}
	if expensive == 0 || cacheable == 0 {
		t.Fatalf("ví dụ mẫu không dạy được gì: %d step đắt, %d step cache được",
			expensive, cacheable)
	}
}

// The prompt is one Go constant assembled from three pieces. A mistake in the
// splice — the example landing outside the fence, a section lost — is silent:
// the request still succeeds and the answers just get worse.
func TestSystemPromptCarriesItsParts(t *testing.T) {
	for _, want := range []string{
		"5 to 8 steps",                     // scale
		"Reference timings",                // consistent seconds
		"keep everything they did not ask", // the follow-up-turn rule
		"docker-build",                     // the worked example was spliced in
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
}
