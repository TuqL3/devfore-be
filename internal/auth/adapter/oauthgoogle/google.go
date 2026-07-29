package oauthgoogle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/devforge/be/internal/auth/domain"
)

type Google struct {
	cfg *oauth2.Config
}

func New(clientID, clientSecret, redirectURL string) *Google {
	return &Google{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (g *Google) Enabled() bool {
	return g.cfg.ClientID != "" && g.cfg.ClientSecret != ""
}

func (g *Google) AuthCodeURL(state string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

type googleUser struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (g *Google) Exchange(ctx context.Context, code string) (domain.GoogleProfile, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return domain.GoogleProfile{}, fmt.Errorf("oauth exchange: %w", err)
	}
	client := g.cfg.Client(ctx, tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return domain.GoogleProfile{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return domain.GoogleProfile{}, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var gu googleUser
	if err := json.NewDecoder(resp.Body).Decode(&gu); err != nil {
		return domain.GoogleProfile{}, err
	}
	if gu.ID == "" || gu.Email == "" {
		return domain.GoogleProfile{}, errors.New("incomplete google profile")
	}
	return domain.GoogleProfile{
		ProviderID: gu.ID,
		Email:      gu.Email,
		Name:       gu.Name,
		AvatarURL:  gu.Picture,
	}, nil
}
