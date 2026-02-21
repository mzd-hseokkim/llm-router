// Package tenant provides request-scoped tenant (org/team) context helpers.
package tenant

import (
	"context"

	"github.com/google/uuid"
)

// OrgIDCtxKey and TeamIDCtxKey are string context keys shared with the rbac
// package. String keys allow both packages to read the same values without
// creating an import cycle (tenant already imports rbac).
const (
	OrgIDCtxKey  = "llm-router:org_id"
	TeamIDCtxKey = "llm-router:team_id"
)

// TenantContext holds the resolved org and team for a request.
type TenantContext struct {
	OrgID  uuid.UUID
	TeamID uuid.UUID // zero value if not scoped to a team
}

// WithTenant stores the tenant context in ctx.
func WithTenant(ctx context.Context, tc TenantContext) context.Context {
	ctx = context.WithValue(ctx, OrgIDCtxKey, tc.OrgID.String())
	ctx = context.WithValue(ctx, TeamIDCtxKey, tc.TeamID.String())
	return context.WithValue(ctx, tenantCtxKey{}, tc)
}

// FromContext retrieves the TenantContext. Returns the zero value if not set.
func FromContext(ctx context.Context) TenantContext {
	tc, _ := ctx.Value(tenantCtxKey{}).(TenantContext)
	return tc
}

// OrgIDString returns the org_id as a string (for the rbac package).
func OrgIDString(ctx context.Context) string {
	v, _ := ctx.Value(OrgIDCtxKey).(string)
	return v
}

// TeamIDString returns the team_id as a string (for the rbac package).
func TeamIDString(ctx context.Context) string {
	v, _ := ctx.Value(TeamIDCtxKey).(string)
	return v
}

// tenantCtxKey is an unexported composite type to avoid collisions.
type tenantCtxKey struct{}
