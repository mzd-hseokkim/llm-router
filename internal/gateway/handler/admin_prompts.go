package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/prompt"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminPromptsHandler serves all /admin/prompts/* endpoints.
type AdminPromptsHandler struct {
	svc   *prompt.Service
	store *pgstore.PromptStore
}

// NewAdminPromptsHandler returns a new handler.
func NewAdminPromptsHandler(svc *prompt.Service, store *pgstore.PromptStore) *AdminPromptsHandler {
	return &AdminPromptsHandler{svc: svc, store: store}
}

// List handles GET /admin/prompts with optional pagination:
//
//	team_id — filter by team UUID (optional)
//	page    — 1-based page number, default: 1 (if omitted without limit, returns all)
//	limit   — page size (1–1000), default: 100 when page is provided
func (h *AdminPromptsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var teamID *string
	if t := q.Get("team_id"); t != "" {
		teamID = &t
	}

	pageStr := q.Get("page")
	limitStr := q.Get("limit")

	// No pagination params: return all (backward compat).
	if pageStr == "" && limitStr == "" {
		rows, err := h.store.ListPrompts(r.Context(), teamID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rows})
		return
	}

	limit := 100
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 1000", "invalid_request_error", "")
			return
		}
		limit = n
	}

	page := 1
	if pageStr != "" {
		n, err := strconv.Atoi(pageStr)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "page must be >= 1", "invalid_request_error", "")
			return
		}
		page = n
	}
	offset := (page - 1) * limit

	total, err := h.store.CountPrompts(r.Context(), teamID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}

	rows, err := h.store.ListPromptsPage(r.Context(), teamID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  rows,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Create handles POST /admin/prompts.
func (h *AdminPromptsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req prompt.CreatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	if req.Slug == "" || req.Name == "" || req.Template == "" {
		writeError(w, http.StatusBadRequest, "slug, name, and template are required", "invalid_request_error", "")
		return
	}

	p, ver, err := h.svc.CreatePrompt(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"prompt": p, "version": ver})
}

// Get handles GET /admin/prompts/{slug} — returns the active version.
func (h *AdminPromptsHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ver, err := h.svc.GetActive(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "not_found_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": p, "version": ver})
}

// ListVersions handles GET /admin/prompts/{slug}/versions.
func (h *AdminPromptsHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, err := h.store.GetPromptBySlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "not_found_error", "")
		return
	}
	versions, err := h.store.ListVersions(r.Context(), p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": versions})
}

// PublishVersion handles POST /admin/prompts/{slug}/versions.
func (h *AdminPromptsHandler) PublishVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var req prompt.PublishVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "invalid_request_error", "")
		return
	}
	if req.Version == "" || req.Template == "" {
		writeError(w, http.StatusBadRequest, "version and template are required", "invalid_request_error", "")
		return
	}

	ver, err := h.svc.PublishVersion(r.Context(), slug, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, ver)
}

// GetVersion handles GET /admin/prompts/{slug}/versions/{version}.
func (h *AdminPromptsHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	version := chi.URLParam(r, "version")

	p, err := h.store.GetPromptBySlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "not_found_error", "")
		return
	}
	ver, err := h.store.GetVersion(r.Context(), p.ID, version)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "not_found_error", "")
		return
	}
	writeJSON(w, http.StatusOK, ver)
}

// Rollback handles POST /admin/prompts/{slug}/rollback/{version}.
func (h *AdminPromptsHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	version := chi.URLParam(r, "version")

	if err := h.svc.Rollback(r.Context(), slug, version); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "api_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "rolled back to " + version})
}

// Render handles POST /admin/prompts/{slug}/render.
func (h *AdminPromptsHandler) Render(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body struct {
		Variables map[string]string `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error", "")
		return
	}
	rendered, tokens, err := h.svc.RenderActive(r.Context(), slug, body.Variables)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error(), "render_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rendered":    rendered,
		"token_count": tokens,
	})
}

// Diff handles GET /admin/prompts/{slug}/diff?from=X&to=Y.
func (h *AdminPromptsHandler) Diff(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "from and to query params are required", "invalid_request_error", "")
		return
	}

	fromTmpl, toTmpl, err := h.svc.Diff(r.Context(), slug, from, to)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error(), "not_found_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": map[string]string{"version": from, "template": fromTmpl},
		"to":   map[string]string{"version": to, "template": toTmpl},
	})
}
