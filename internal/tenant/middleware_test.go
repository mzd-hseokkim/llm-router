package tenant_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/llm-router/gateway/internal/auth/rbac"
	"github.com/llm-router/gateway/internal/tenant"
)

// resolverFunc adapts a function to the tenant.Resolver interface.
type resolverFunc func(ctx context.Context, userID uuid.UUID) (uuid.UUID, uuid.UUID, error)

func (f resolverFunc) ResolveForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	return f(ctx, userID)
}

func injectUser(req *http.Request, userID uuid.UUID) *http.Request {
	ui := &rbac.UserInfo{ID: userID}
	return rbac.InjectUser(req, ui)
}

// TestMiddleware_ResolverError_Returns500 is the regression test for Fix 6.
// Before the fix, a DB error in ResolveForUser caused the request to continue
// without tenant context (silent pass-through), bypassing org isolation.
// After the fix, it returns 500 (fail-closed).
func TestMiddleware_ResolverError_Returns500(t *testing.T) {
	resolver := resolverFunc(func(_ context.Context, _ uuid.UUID) (uuid.UUID, uuid.UUID, error) {
		return uuid.UUID{}, uuid.UUID{}, errors.New("db connection lost")
	})

	mw := tenant.Middleware(resolver)

	innerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	req = injectUser(req, uuid.New())

	rw := httptest.NewRecorder()
	mw(inner).ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on resolver error, got %d", rw.Code)
	}
	if innerCalled {
		t.Error("inner handler must not be called when resolver fails")
	}
}

// TestMiddleware_NoUser_PassesThrough verifies that unauthenticated requests
// (no UserInfo in context) still pass through without tenant injection.
func TestMiddleware_NoUser_PassesThrough(t *testing.T) {
	resolver := resolverFunc(func(_ context.Context, _ uuid.UUID) (uuid.UUID, uuid.UUID, error) {
		// Should not be called for unauthenticated requests.
		return uuid.UUID{}, uuid.UUID{}, errors.New("unexpected call")
	})

	mw := tenant.Middleware(resolver)

	innerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		// No tenant context expected.
		tc := tenant.FromContext(r.Context())
		if tc.OrgID != (uuid.UUID{}) {
			t.Errorf("expected empty TenantContext for unauthenticated request, got %+v", tc)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/v1/models", nil)
	// No rbac.InjectUser — simulates an unauthenticated (or virtual-key) request.

	rw := httptest.NewRecorder()
	mw(inner).ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("expected 200 for unauthenticated request, got %d", rw.Code)
	}
	if !innerCalled {
		t.Error("inner handler must be called for unauthenticated requests")
	}
}

// TestMiddleware_Success_InjectsTenantContext verifies the happy path:
// TenantContext is injected and both string keys readable by rbac are set.
func TestMiddleware_Success_InjectsTenantContext(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()

	resolver := resolverFunc(func(_ context.Context, _ uuid.UUID) (uuid.UUID, uuid.UUID, error) {
		return orgID, teamID, nil
	})

	mw := tenant.Middleware(resolver)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc := tenant.FromContext(r.Context())
		if tc.OrgID != orgID {
			t.Errorf("OrgID: got %v, want %v", tc.OrgID, orgID)
		}
		if tc.TeamID != teamID {
			t.Errorf("TeamID: got %v, want %v", tc.TeamID, teamID)
		}

		// Also verify the rbac-readable string keys are present.
		if got := tenant.OrgIDString(r.Context()); got != orgID.String() {
			t.Errorf("OrgIDString: got %q, want %q", got, orgID.String())
		}
		if got := tenant.TeamIDString(r.Context()); got != teamID.String() {
			t.Errorf("TeamIDString: got %q, want %q", got, teamID.String())
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	req = injectUser(req, uuid.New())

	rw := httptest.NewRecorder()
	mw(inner).ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rw.Code)
	}
}
