package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth/oauth"
	"github.com/llm-router/gateway/internal/auth/rbac"
	"github.com/llm-router/gateway/internal/auth/session"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

const sessionCookie = "llm_session"

// AuthHandler handles OAuth 2.0 login, callback, logout, and /auth/me.
type AuthHandler struct {
	providers  map[string]oauth.Provider
	sessions   *session.Store
	orgStore   *pgstore.OrgStore
	roleStore  *pgstore.RoleStore
	authorizer *rbac.Authorizer
	logger     *slog.Logger
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(
	providers []oauth.Provider,
	sessions *session.Store,
	orgStore *pgstore.OrgStore,
	roleStore *pgstore.RoleStore,
	authorizer *rbac.Authorizer,
	logger *slog.Logger,
) *AuthHandler {
	pm := make(map[string]oauth.Provider, len(providers))
	for _, p := range providers {
		pm[p.Name()] = p
	}
	return &AuthHandler{
		providers:  pm,
		sessions:   sessions,
		orgStore:   orgStore,
		roleStore:  roleStore,
		authorizer: authorizer,
		logger:     logger,
	}
}

// Providers lists the active OAuth providers.
// GET /auth/providers
func (h *AuthHandler) Providers(w http.ResponseWriter, r *http.Request) {
	names := make([]string, 0, len(h.providers))
	for name := range h.providers {
		names = append(names, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": names})
}

// Login initiates the OAuth authorization code flow.
// GET /auth/login?provider=google
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		providerName = "google"
	}

	p, ok := h.providers[providerName]
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown provider %q"}`, providerName), http.StatusBadRequest)
		return
	}

	state, err := oauth.GenerateState()
	if err != nil {
		h.logger.Error("generate oauth state", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if err := h.sessions.StoreState(r.Context(), state, providerName); err != nil {
		h.logger.Error("store oauth state", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	redirectURL := p.AuthCodeURL(state)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Callback handles the OAuth provider redirect.
// GET /auth/callback?code=xxx&state=xxx&provider=google
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	providerParam := r.URL.Query().Get("provider")

	if state == "" || code == "" {
		http.Error(w, `{"error":"missing state or code"}`, http.StatusBadRequest)
		return
	}

	// Verify CSRF state
	providerName, err := h.sessions.VerifyState(r.Context(), state)
	if err != nil {
		http.Error(w, `{"error":"invalid oauth state"}`, http.StatusBadRequest)
		return
	}

	// Allow provider override from query param (some IdPs don't pass it back)
	if providerParam != "" {
		providerName = providerParam
	}

	p, ok := h.providers[providerName]
	if !ok {
		http.Error(w, `{"error":"unknown provider"}`, http.StatusBadRequest)
		return
	}

	// Exchange code for user profile
	profile, err := p.Exchange(r.Context(), code)
	if err != nil {
		h.logger.Error("oauth exchange failed", "provider", providerName, "error", err)
		http.Error(w, `{"error":"authentication failed"}`, http.StatusUnauthorized)
		return
	}

	// JIT provisioning: find or create user
	user, err := h.findOrCreateUser(r, profile)
	if err != nil {
		h.logger.Error("jit provisioning failed", "email", profile.Email, "error", err)
		http.Error(w, `{"error":"user provisioning failed"}`, http.StatusInternalServerError)
		return
	}

	// Create session
	sess, err := h.sessions.Create(r.Context(), user.ID, providerName)
	if err != nil {
		h.logger.Error("create session failed", "error", err)
		http.Error(w, `{"error":"session creation failed"}`, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		Expires:  sess.ExpiresAt,
	})

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

// Me returns the current user's profile.
// GET /auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	ui := rbac.UserFromContext(r.Context())
	if ui == nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":    ui.ID,
		"email": ui.Email,
		"roles": ui.Roles,
	})
}

// Logout invalidates the session.
// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		_ = h.sessions.Delete(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// SessionMiddleware validates the session cookie and injects UserInfo into the context.
func (h *AuthHandler) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		sess, err := h.sessions.Get(r.Context(), cookie.Value)
		if err != nil || sess == nil || sess.IsExpired() {
			next.ServeHTTP(w, r)
			return
		}

		// Auto-renew near expiry and refresh the browser cookie so its
		// Expires matches the extended Redis TTL.
		if sess.NeedsRenewal() {
			if renewed, err := h.sessions.Renew(r.Context(), sess.ID); err == nil && renewed != nil {
				http.SetCookie(w, &http.Cookie{
					Name:     sessionCookie,
					Value:    sess.ID,
					Path:     "/",
					HttpOnly: true,
					Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
					SameSite: http.SameSiteStrictMode,
					Expires:  renewed.ExpiresAt,
				})
				sess = renewed
			}
		}

		// Load user roles and inject UserInfo
		ui, err := h.authorizer.GetUserInfo(r.Context(), sess.UserID)
		if err != nil {
			h.logger.Warn("load user info for session", "error", err)
			next.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, rbac.InjectUser(r, ui))
	})
}

// findOrCreateUser implements JIT provisioning.
func (h *AuthHandler) findOrCreateUser(r *http.Request, profile *oauth.UserProfile) (*pgstore.User, error) {
	ctx := r.Context()

	user, err := h.orgStore.GetUserByEmail(ctx, profile.Email)
	if err == nil {
		return user, nil
	}
	if err != pgstore.ErrUserNotFound {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	// Create new user (no org/team assignment yet — admin must assign)
	user, err = h.orgStore.CreateUser(ctx, nil, nil, profile.Email)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Assign default 'developer' role
	role, err := h.roleStore.GetRoleByName(ctx, "developer", nil)
	if err == nil {
		_ = h.roleStore.AssignRole(ctx, user.ID, role.ID, nil, nil)
	}

	h.logger.Info("jit provisioned new user", "email", profile.Email, "id", user.ID)
	return user, nil
}

// AdminRolesHandler handles role CRUD for the admin API.
type AdminRolesHandler struct {
	roleStore  *pgstore.RoleStore
	authorizer *rbac.Authorizer
}

func NewAdminRolesHandler(roleStore *pgstore.RoleStore, authorizer *rbac.Authorizer) *AdminRolesHandler {
	return &AdminRolesHandler{roleStore: roleStore, authorizer: authorizer}
}

// ListRoles lists all roles visible to the caller's org.
// GET /admin/roles
func (h *AdminRolesHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.roleStore.ListRoles(r.Context(), nil)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

// AssignRole assigns a role to a user.
// POST /admin/users/{user_id}/roles
func (h *AdminRolesHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	userIDStr := strings.TrimPrefix(r.URL.Path, "/admin/users/")
	userIDStr = strings.TrimSuffix(userIDStr, "/roles")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		RoleName string `json:"role_name"`
		OrgID    string `json:"org_id"`
		TeamID   string `json:"team_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	var orgID, teamID *uuid.UUID
	if req.OrgID != "" {
		id, err := uuid.Parse(req.OrgID)
		if err == nil {
			orgID = &id
		}
	}
	if req.TeamID != "" {
		id, err := uuid.Parse(req.TeamID)
		if err == nil {
			teamID = &id
		}
	}

	role, err := h.roleStore.GetRoleByName(r.Context(), req.RoleName, orgID)
	if err != nil {
		http.Error(w, `{"error":"role not found"}`, http.StatusNotFound)
		return
	}

	// Privilege escalation guard: only super_admin can assign roles that carry
	// PermAll or system-management permissions.
	callerUI := rbac.UserFromContext(r.Context())
	if !callerUI.HasPermission(rbac.PermAll, "", "") {
		for _, p := range role.Permissions {
			if p == rbac.PermAll || p == rbac.PermManageSystem || p == rbac.PermManageProviders {
				http.Error(w, `{"error":"forbidden: insufficient privileges to assign this role"}`, http.StatusForbidden)
				return
			}
		}
	}

	if err := h.roleStore.AssignRole(r.Context(), userID, role.ID, orgID, teamID); err != nil {
		http.Error(w, `{"error":"assign role failed"}`, http.StatusInternalServerError)
		return
	}

	h.authorizer.InvalidateCache(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
