package prompt

import (
	"context"
	"encoding/json"
	"fmt"

	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// Service provides high-level prompt management operations.
type Service struct {
	store *pgstore.PromptStore
}

// NewService returns a prompt Service backed by store.
func NewService(store *pgstore.PromptStore) *Service {
	return &Service{store: store}
}

// CreatePrompt creates a new prompt with an initial draft version.
func (s *Service) CreatePrompt(ctx context.Context, req CreatePromptRequest) (*pgstore.PromptRow, *pgstore.PromptVersionRow, error) {
	prompt, err := s.store.CreatePrompt(ctx,
		req.Slug, req.Name, req.Description, orDefault(req.Visibility, "team"),
		req.TeamID, req.CreatedBy)
	if err != nil {
		return nil, nil, fmt.Errorf("create prompt: %w", err)
	}

	vars, _ := json.Marshal(req.Variables)
	params, _ := json.Marshal(req.Parameters)

	ver, err := s.store.CreateVersion(ctx,
		prompt.ID, orDefault(req.Version, "1.0.0"), "active",
		req.Template, req.Model, req.Changelog, vars, params, req.CreatedBy)
	if err != nil {
		return nil, nil, fmt.Errorf("create prompt version: %w", err)
	}

	return prompt, ver, nil
}

// PublishVersion creates a new version for an existing prompt and activates it.
func (s *Service) PublishVersion(ctx context.Context, slug string, req PublishVersionRequest) (*pgstore.PromptVersionRow, error) {
	prompt, err := s.store.GetPromptBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("prompt not found: %w", err)
	}

	vars, _ := json.Marshal(req.Variables)
	params, _ := json.Marshal(req.Parameters)

	ver, err := s.store.CreateVersion(ctx,
		prompt.ID, req.Version, "active",
		req.Template, req.Model, req.Changelog, vars, params, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("publish version: %w", err)
	}

	// Activate the new version (and deprecate the old active one).
	if err := s.store.SetVersionStatus(ctx, prompt.ID, ver.Version, "active"); err != nil {
		return nil, fmt.Errorf("activate version: %w", err)
	}

	return ver, nil
}

// Rollback sets the given version as active, deprecating the current one.
func (s *Service) Rollback(ctx context.Context, slug, version string) error {
	prompt, err := s.store.GetPromptBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("prompt not found: %w", err)
	}
	return s.store.SetVersionStatus(ctx, prompt.ID, version, "active")
}

// GetActive returns the active version of the prompt with the given slug.
func (s *Service) GetActive(ctx context.Context, slug string) (*pgstore.PromptRow, *pgstore.PromptVersionRow, error) {
	prompt, err := s.store.GetPromptBySlug(ctx, slug)
	if err != nil {
		return nil, nil, err
	}
	ver, err := s.store.GetActiveVersion(ctx, prompt.ID)
	if err != nil {
		return nil, nil, err
	}
	return prompt, ver, nil
}

// RenderActive looks up the active version of slug, renders it with values,
// and returns the rendered text and token count.
func (s *Service) RenderActive(ctx context.Context, slug string, values map[string]string) (string, int, error) {
	_, ver, err := s.GetActive(ctx, slug)
	if err != nil {
		return "", 0, err
	}

	var defs []VariableDef
	_ = json.Unmarshal(ver.Variables, &defs)

	rendered, err := Render(ver.Template, defs, values)
	if err != nil {
		return "", 0, err
	}
	return rendered, CountTokens(rendered), nil
}

// Diff returns the templates of two versions so the caller can compare them.
func (s *Service) Diff(ctx context.Context, slug, fromVer, toVer string) (from, to string, err error) {
	prompt, err := s.store.GetPromptBySlug(ctx, slug)
	if err != nil {
		return "", "", fmt.Errorf("prompt not found: %w", err)
	}
	fv, err := s.store.GetVersion(ctx, prompt.ID, fromVer)
	if err != nil {
		return "", "", fmt.Errorf("from version: %w", err)
	}
	tv, err := s.store.GetVersion(ctx, prompt.ID, toVer)
	if err != nil {
		return "", "", fmt.Errorf("to version: %w", err)
	}
	return fv.Template, tv.Template, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// --- Request types ---

type CreatePromptRequest struct {
	Slug        string            `json:"slug"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	TeamID      *string           `json:"team_id"`
	Visibility  string            `json:"visibility"`
	Template    string            `json:"template"`
	Variables   []VariableDef     `json:"variables"`
	Parameters  map[string]any    `json:"parameters"`
	Model       string            `json:"model"`
	Version     string            `json:"version"`
	Changelog   string            `json:"changelog"`
	CreatedBy   *string           `json:"created_by"`
}

type PublishVersionRequest struct {
	Version    string         `json:"version"`
	Template   string         `json:"template"`
	Variables  []VariableDef  `json:"variables"`
	Parameters map[string]any `json:"parameters"`
	Model      string         `json:"model"`
	Changelog  string         `json:"changelog"`
	CreatedBy  *string        `json:"created_by"`
}
