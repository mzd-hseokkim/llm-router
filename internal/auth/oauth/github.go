package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// GitHubProvider implements Provider for GitHub OAuth.
type GitHubProvider struct {
	cfg    Config
	oauth2 *oauth2.Config
}

// NewGitHub creates a GitHub OAuth provider.
func NewGitHub(cfg Config) *GitHubProvider {
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"read:user", "user:email"}
	}
	o2 := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     github.Endpoint,
	}
	return &GitHubProvider{cfg: cfg, oauth2: o2}
}

func (g *GitHubProvider) Name() string { return "github" }

func (g *GitHubProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return g.oauth2.AuthCodeURL(state, opts...)
}

func (g *GitHubProvider) Exchange(ctx context.Context, code string) (*UserProfile, error) {
	token, err := g.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}

	client := g.oauth2.Client(ctx, token)

	// Fetch user info
	userResp, err := client.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("github user api: %w", err)
	}
	defer userResp.Body.Close()

	body, _ := io.ReadAll(userResp.Body)
	var userInfo struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}

	email := userInfo.Email
	if email == "" {
		// GitHub may not expose email directly; fetch from /user/emails
		email, err = g.fetchPrimaryEmail(client)
		if err != nil {
			email = fmt.Sprintf("%s@users.noreply.github.com", userInfo.Login)
		}
	}

	return &UserProfile{
		ProviderID: fmt.Sprintf("%d", userInfo.ID),
		Email:      email,
		Name:       userInfo.Name,
	}, nil
}

func (g *GitHubProvider) fetchPrimaryEmail(client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no primary email found")
}
