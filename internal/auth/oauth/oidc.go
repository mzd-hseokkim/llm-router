package oauth

import (
	"context"
	"fmt"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCProvider implements Provider for any OIDC-compliant IdP (Okta, Microsoft, Keycloak, etc.).
type OIDCProvider struct {
	name     string
	cfg      Config
	oauth2   *oauth2.Config
	verifier *gooidc.IDTokenVerifier
}

// NewOIDC creates a generic OIDC provider by discovering the IdP's metadata.
// issuerURL is the OIDC issuer (e.g. "https://company.okta.com").
func NewOIDC(ctx context.Context, name string, cfg Config, issuerURL string) (*OIDCProvider, error) {
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discover %s: %w", issuerURL, err)
	}

	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "email", "profile"}
	}

	o2 := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     provider.Endpoint(),
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	return &OIDCProvider{
		name:     name,
		cfg:      cfg,
		oauth2:   o2,
		verifier: verifier,
	}, nil
}

func (p *OIDCProvider) Name() string { return p.name }

func (p *OIDCProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return p.oauth2.AuthCodeURL(state, opts...)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*UserProfile, error) {
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc token exchange (%s): %w", p.name, err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("oidc: no id_token in response from %s", p.name)
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc verify id_token (%s): %w", p.name, err)
	}

	var claims struct {
		Sub    string   `json:"sub"`
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		Groups []string `json:"groups"` // Okta, Azure AD, Keycloak
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc parse claims (%s): %w", p.name, err)
	}

	return &UserProfile{
		ProviderID: claims.Sub,
		Email:      claims.Email,
		Name:       claims.Name,
		Groups:     claims.Groups,
	}, nil
}

// MapGroupsToRoles maps IdP group names to Gateway role strings using the provider config.
func MapGroupsToRoles(groups []string, mapping map[string]string) []string {
	roleSet := make(map[string]bool)
	for _, g := range groups {
		if role, ok := mapping[g]; ok {
			roleSet[role] = true
		}
	}
	roles := make([]string, 0, len(roleSet))
	for r := range roleSet {
		roles = append(roles, r)
	}
	return roles
}
