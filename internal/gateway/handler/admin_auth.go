package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/ratelimit"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

// AdminAuthHandler handles admin login, password change, and profile endpoints.
type AdminAuthHandler struct {
	store       *pgstore.AdminCredentialStore
	jwtSvc      *auth.JWTService
	rateLimiter *ratelimit.RedisLimiter
	logger      *slog.Logger
}

// NewAdminAuthHandler creates an AdminAuthHandler.
// rateLimiter may be nil (disables login rate limiting, useful in tests).
func NewAdminAuthHandler(store *pgstore.AdminCredentialStore, jwtSvc *auth.JWTService, rateLimiter *ratelimit.RedisLimiter, logger *slog.Logger) *AdminAuthHandler {
	return &AdminAuthHandler{store: store, jwtSvc: jwtSvc, rateLimiter: rateLimiter, logger: logger}
}

// Login handles POST /admin/auth/login.
func (h *AdminAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Rate limit login attempts per username to prevent brute-force.
	// Keyed by "admin_login:<username>" — 10 attempts per 15 minutes globally.
	// Checked before password decode so the username "admin" is the implicit key.
	if h.rateLimiter != nil {
		res, err := h.rateLimiter.Allow(r.Context(), "admin_login:admin", 10, 1, 15*time.Minute)
		if err != nil {
			h.logger.Warn("admin login: rate limiter error", "error", err)
			// Fail open — don't block login if Redis is unavailable.
		} else if !res.Allowed {
			w.Header().Set("Retry-After", res.ResetAt.Format(http.TimeFormat))
			writeError(w, http.StatusTooManyRequests, "too many login attempts — try again later", "invalid_request_error", "rate_limit_exceeded")
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required", "invalid_request_error", "bad_request")
		return
	}

	cred, err := h.store.GetByUsername(r.Context(), "admin")
	if err != nil {
		if errors.Is(err, pgstore.ErrAdminCredNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid credentials", "invalid_request_error", "invalid_api_key")
			return
		}
		h.logger.Error("admin login: db error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials", "invalid_request_error", "invalid_api_key")
		return
	}

	token, err := h.jwtSvc.GenerateToken(cred.Username, cred.PasswordChanged)
	if err != nil {
		h.logger.Error("admin login: token generation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":            token,
		"password_changed": cred.PasswordChanged,
	})
}

// ChangePassword handles POST /admin/auth/change-password (JWT required).
func (h *AdminAuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetAdminClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid_request_error", "invalid_api_key")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request_error", "bad_request")
		return
	}
	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters", "invalid_request_error", "bad_request")
		return
	}

	cred, err := h.store.GetByUsername(r.Context(), claims.Username)
	if err != nil {
		h.logger.Error("change-password: db error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(body.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect", "invalid_request_error", "invalid_api_key")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("change-password: bcrypt error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	if err := h.store.UpdatePassword(r.Context(), claims.Username, string(newHash)); err != nil {
		h.logger.Error("change-password: update error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	token, err := h.jwtSvc.GenerateToken(claims.Username, true)
	if err != nil {
		h.logger.Error("change-password: token generation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error", "api_error", "internal_error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":            token,
		"password_changed": true,
	})
}

// Me handles GET /admin/auth/me (JWT required).
func (h *AdminAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetAdminClaims(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid_request_error", "invalid_api_key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":         claims.Username,
		"password_changed": claims.PasswordChanged,
	})
}
