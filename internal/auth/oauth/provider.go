// Package oauth implements OAuth 2.0 / OIDC provider integrations.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
)

// UserProfile is the normalized user data returned by any OAuth provider.
type UserProfile struct {
	ProviderID string   // unique ID within the provider
	Email      string
	Name       string
	Groups     []string // IdP groups (for role mapping)
}

// Provider is the interface all OAuth/OIDC adapters must satisfy.
type Provider interface {
	// Name returns the provider identifier ("google", "github", "okta", etc.)
	Name() string

	// AuthCodeURL returns the provider's OAuth redirect URL with state and PKCE params.
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string

	// Exchange converts an authorization code to tokens and returns the user profile.
	Exchange(ctx context.Context, code string) (*UserProfile, error)
}

// Config holds common OAuth provider settings.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	// GroupRoleMapping maps IdP group names to Gateway roles.
	GroupRoleMapping map[string]string
}

// GenerateState creates a cryptographically random state token for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CallbackURL builds the callback URL from a request.
func CallbackURL(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/callback?provider=%s", scheme, r.Host, provider)
}
