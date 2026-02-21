package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminOrgsHandler handles CRUD for organizations, teams, and users.
type AdminOrgsHandler struct {
	store *pgstore.OrgStore
}

// NewAdminOrgsHandler creates the handler.
func NewAdminOrgsHandler(store *pgstore.OrgStore) *AdminOrgsHandler {
	return &AdminOrgsHandler{store: store}
}

// --- Organizations ---

func (h *AdminOrgsHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "")
		return
	}
	org, err := h.store.CreateOrg(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create organization", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (h *AdminOrgsHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.store.ListOrgs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list organizations", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": orgs})
}

func (h *AdminOrgsHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	org, err := h.store.GetOrg(r.Context(), id)
	if errors.Is(err, pgstore.ErrOrgNotFound) {
		writeError(w, http.StatusNotFound, "organization not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get organization", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *AdminOrgsHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "")
		return
	}
	org, err := h.store.UpdateOrg(r.Context(), id, req.Name)
	if errors.Is(err, pgstore.ErrOrgNotFound) {
		writeError(w, http.StatusNotFound, "organization not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update organization", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

// --- Teams ---

func (h *AdminOrgsHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgID string `json:"org_id"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "")
		return
	}

	var orgID *uuid.UUID
	if req.OrgID != "" {
		id, err := uuid.Parse(req.OrgID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid org_id", "invalid_request_error", "")
			return
		}
		orgID = &id
	}

	team, err := h.store.CreateTeam(r.Context(), orgID, req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create team", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

func (h *AdminOrgsHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	var orgID *uuid.UUID
	if s := r.URL.Query().Get("org_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid org_id", "invalid_request_error", "")
			return
		}
		orgID = &id
	}

	teams, err := h.store.ListTeams(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list teams", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": teams})
}

func (h *AdminOrgsHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	team, err := h.store.GetTeam(r.Context(), id)
	if errors.Is(err, pgstore.ErrTeamNotFound) {
		writeError(w, http.StatusNotFound, "team not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get team", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *AdminOrgsHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required", "invalid_request_error", "")
		return
	}
	team, err := h.store.UpdateTeam(r.Context(), id, req.Name)
	if errors.Is(err, pgstore.ErrTeamNotFound) {
		writeError(w, http.StatusNotFound, "team not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update team", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, team)
}

// --- Users ---

func (h *AdminOrgsHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgID  string `json:"org_id"`
		TeamID string `json:"team_id"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required", "invalid_request_error", "")
		return
	}

	var orgID *uuid.UUID
	if req.OrgID != "" {
		id, err := uuid.Parse(req.OrgID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid org_id", "invalid_request_error", "")
			return
		}
		orgID = &id
	}

	var teamID *uuid.UUID
	if req.TeamID != "" {
		id, err := uuid.Parse(req.TeamID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_id", "invalid_request_error", "")
			return
		}
		teamID = &id
	}

	user, err := h.store.CreateUser(r.Context(), orgID, teamID, req.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *AdminOrgsHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	var orgID *uuid.UUID
	if s := r.URL.Query().Get("org_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid org_id", "invalid_request_error", "")
			return
		}
		orgID = &id
	}

	users, err := h.store.ListUsers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": users})
}

func (h *AdminOrgsHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	user, err := h.store.GetUser(r.Context(), id)
	if errors.Is(err, pgstore.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AdminOrgsHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req struct {
		Email  string  `json:"email"`
		TeamID *string `json:"team_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required", "invalid_request_error", "")
		return
	}

	var teamID *uuid.UUID
	if req.TeamID != nil && *req.TeamID != "" {
		tid, err := uuid.Parse(*req.TeamID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid team_id", "invalid_request_error", "")
			return
		}
		teamID = &tid
	}

	user, err := h.store.UpdateUser(r.Context(), id, req.Email, teamID)
	if errors.Is(err, pgstore.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found", "invalid_request_error", "not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user", "internal_error", "")
		return
	}
	writeJSON(w, http.StatusOK, user)
}
