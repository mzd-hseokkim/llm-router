package tenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth/rbac"
	"github.com/llm-router/gateway/internal/tenant"
)

// TestWithTenant_ContextKeysReadableByRBAC is the regression test for Fix 1.
//
// Before the fix: tenant.WithTenant stored values under tenant.ctxKey(0/1),
// but rbac.orgTeamFromCtx read under rbac.ctxKey(10/11) — different types,
// so ctx.Value always returned nil, making org-scoped RBAC checks silently pass
// for everyone (global roles) or silently fail (org-scoped roles).
//
// After the fix: both packages use the same string constants
// "llm-router:org_id" / "llm-router:team_id".
func TestWithTenant_ContextKeysReadableByRBAC(t *testing.T) {
	orgID := uuid.New()

	// User with an org-scoped org_admin role (not global).
	ui := &rbac.UserInfo{
		ID: uuid.New(),
		Roles: []rbac.UserRole{{
			Role:        rbac.RoleOrgAdmin,
			OrgID:       orgID.String(), // scoped to one specific org
			Permissions: rbac.DefaultRolePermissions[rbac.RoleOrgAdmin],
		}},
	}

	mw := rbac.RequirePermission(nil, rbac.PermCreateKey)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(okHandler)

	t.Run("matching org grants access", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req = rbac.InjectUser(req, ui)
		req = req.WithContext(tenant.WithTenant(req.Context(), tenant.TenantContext{OrgID: orgID}))

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusOK {
			t.Errorf("expected 200 for matching org, got %d (RBAC scope context key mismatch?)", rw.Code)
		}
	})

	t.Run("different org denies access", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req = rbac.InjectUser(req, ui)
		// Put a DIFFERENT org in tenant context — permission must be denied.
		req = req.WithContext(tenant.WithTenant(req.Context(), tenant.TenantContext{OrgID: uuid.New()}))

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusForbidden {
			t.Errorf("expected 403 for wrong org, got %d", rw.Code)
		}
	})

	t.Run("no tenant context falls back to empty scope", func(t *testing.T) {
		// Without tenant context, orgID in scope check is "". The org-scoped role
		// (OrgID != "") will not match "", so access is correctly denied.
		req := httptest.NewRequest("GET", "/test", nil)
		req = rbac.InjectUser(req, ui)
		// No tenant.WithTenant call.

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		if rw.Code != http.StatusForbidden {
			t.Errorf("expected 403 without tenant context, got %d", rw.Code)
		}
	})
}

// TestWithTenant_ExportedKeysMatchStoredValues checks the contract directly:
// the exported string constants must retrieve the values stored by WithTenant.
func TestWithTenant_ExportedKeysMatchStoredValues(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()

	ctx := tenant.WithTenant(context.Background(), tenant.TenantContext{
		OrgID:  orgID,
		TeamID: teamID,
	})

	if got, _ := ctx.Value(tenant.OrgIDCtxKey).(string); got != orgID.String() {
		t.Errorf("OrgIDCtxKey: got %q, want %q", got, orgID.String())
	}
	if got, _ := ctx.Value(tenant.TeamIDCtxKey).(string); got != teamID.String() {
		t.Errorf("TeamIDCtxKey: got %q, want %q", got, teamID.String())
	}

	// Helper functions must agree with the raw key access.
	if got := tenant.OrgIDString(ctx); got != orgID.String() {
		t.Errorf("OrgIDString: got %q, want %q", got, orgID.String())
	}
	if got := tenant.TeamIDString(ctx); got != teamID.String() {
		t.Errorf("TeamIDString: got %q, want %q", got, teamID.String())
	}
}
