package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleProvider implements Provider for Google Workspace OAuth.
type GoogleProvider struct {
	cfg    Config
	oauth2 *oauth2.Config
}

// NewGoogle creates a Google OAuth provider.
func NewGoogle(cfg Config) *GoogleProvider {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "email", "profile"}
	}
	o2 := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     google.Endpoint,
	}
	return &GoogleProvider{cfg: cfg, oauth2: o2}
}

func (g *GoogleProvider) Name() string { return "google" }

func (g *GoogleProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return g.oauth2.AuthCodeURL(state, opts...)
}

func (g *GoogleProvider) Exchange(ctx context.Context, code string) (*UserProfile, error) {
	token, err := g.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google token exchange: %w", err)
	}

	client := g.oauth2.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read google userinfo: %w", err)
	}

	var info struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}

	return &UserProfile{
		ProviderID: info.Sub,
		Email:      info.Email,
		Name:       info.Name,
	}, nil
}
