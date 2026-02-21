package tenant

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth/rbac"
)

// Resolver can look up org/team info given a user or virtual key context.
// Implement this at the call site to avoid import cycles.
type Resolver interface {
	// ResolveForUser returns the primary org and team for a user ID.
	ResolveForUser(ctx context.Context, userID uuid.UUID) (orgID, teamID uuid.UUID, err error)
}

// Middleware injects TenantContext into each request based on the authenticated user.
// It also sets the org/team string values that the rbac package reads.
func Middleware(resolver Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ui := rbac.UserFromContext(r.Context())
			if ui == nil {
				// No authenticated user — skip tenant injection (public route)
				next.ServeHTTP(w, r)
				return
			}

			orgID, teamID, err := resolver.ResolveForUser(r.Context(), ui.ID)
			if err != nil {
				// Proceed without tenant context rather than failing hard
				next.ServeHTTP(w, r)
				return
			}

			tc := TenantContext{OrgID: orgID, TeamID: teamID}
			next.ServeHTTP(w, r.WithContext(WithTenant(r.Context(), tc)))
		})
	}
}
