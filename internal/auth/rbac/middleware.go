package rbac

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const (
	ctxUserInfo ctxKey = iota
)

// InjectUser stores a UserInfo in the request context.
// Call this from your session/auth middleware after identifying the user.
func InjectUser(r *http.Request, ui *UserInfo) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxUserInfo, ui))
}

// UserFromContext retrieves the UserInfo from the request context.
// Returns nil if not present.
func UserFromContext(ctx context.Context) *UserInfo {
	ui, _ := ctx.Value(ctxUserInfo).(*UserInfo)
	return ui
}

// RequirePermission returns a middleware that enforces the given permission.
// It reads the OrgID/TeamID from the tenant context (if available) for scope checks.
func RequirePermission(auth *Authorizer, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ui := UserFromContext(r.Context())
			if ui == nil {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}

			orgID, teamID := orgTeamFromCtx(r.Context())
			if !ui.HasPermission(perm, orgID, teamID) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns a middleware that enforces at least one of the given roles.
func RequireRole(roles ...Role) func(http.Handler) http.Handler {
	roleSet := make(map[Role]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ui := UserFromContext(r.Context())
			if ui == nil {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			for _, ur := range ui.Roles {
				if roleSet[ur.Role] {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		})
	}
}

// orgTeamFromCtx extracts tenant scope from context (set by tenant middleware).
// Returns empty strings if not present.
func orgTeamFromCtx(ctx context.Context) (orgID, teamID string) {
	type tenantCtx interface {
		OrgID() uuid.UUID
		TeamID() uuid.UUID
	}
	// We use string keys to avoid import cycles; tenant middleware sets these.
	if v := ctx.Value(ctxOrgID); v != nil {
		orgID = v.(string)
	}
	if v := ctx.Value(ctxTeamID); v != nil {
		teamID = v.(string)
	}
	return
}

// These keys are shared with the tenant package via this file.
// Using typed constants avoids import cycles between rbac and tenant.
const (
	ctxOrgID  = ctxKey(10)
	ctxTeamID = ctxKey(11)
)
