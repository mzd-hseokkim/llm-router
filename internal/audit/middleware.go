package audit

import (
	"net/http"
	"strings"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// auditableEndpoints maps HTTP method+path-prefix to (eventType, action).
// Patterns are matched by prefix so /admin/keys/{id} is covered by "/admin/keys".
var auditableEndpoints = []struct {
	method    string
	prefix    string
	eventType string
	action    string
}{
	// Virtual keys
	{"POST", "/admin/keys", EventKeyCreated, "create"},
	{"PATCH", "/admin/keys", EventKeyUpdated, "update"},
	{"DELETE", "/admin/keys", EventKeyDeactivated, "delete"},
	{"POST", "/admin/keys/", EventKeyRotated, "rotate"}, // /admin/keys/{id}/regenerate

	// Provider keys
	{"POST", "/admin/provider-keys", EventProviderKeyCreated, "create"},
	{"PUT", "/admin/provider-keys", EventProviderKeyUpdated, "update"},
	{"DELETE", "/admin/provider-keys", EventProviderKeyDeleted, "delete"},

	// Routing
	{"POST", "/admin/routing/rules", EventRoutingRuleCreated, "create"},
	{"PUT", "/admin/routing/rules", EventRoutingRuleUpdated, "update"},
	{"DELETE", "/admin/routing/rules", EventRoutingRuleDeleted, "delete"},
	{"POST", "/admin/routing/reload", EventRoutingReloaded, "reload"},
	{"PUT", "/admin/routing", EventRoutingReloaded, "update"},

	// Users / teams / orgs
	{"POST", "/admin/users", EventUserCreated, "create"},
	{"PUT", "/admin/users", EventUserUpdated, "update"},
	{"DELETE", "/admin/users", EventUserDeleted, "delete"},
	{"POST", "/admin/users/", EventRoleAssigned, "assign_role"}, // /admin/users/{id}/roles

	// Budget
	{"POST", "/admin/budgets", EventBudgetCreated, "create"},
	{"POST", "/admin/budgets/", EventBudgetReset, "reset"},

	// System
	{"POST", "/admin/circuit-breakers", EventCircuitBreakerReset, "reset"},
	{"DELETE", "/admin/cache", EventCacheDeleted, "delete"},
}

// statusRecorder captures the HTTP status code written by the inner handler.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.code = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.code == 0 {
		sr.code = http.StatusOK
	}
	return sr.ResponseWriter.Write(b)
}

func (sr *statusRecorder) status() int {
	if sr.code == 0 {
		return http.StatusOK
	}
	return sr.code
}

// Flush implements http.Flusher so SSE streaming works through this wrapper.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying ResponseWriter so http.ResponseController
// (used in proxy/stream.go for SetWriteDeadline) can reach it.
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

// Middleware returns an HTTP middleware that records admin-API calls to the audit log.
func Middleware(auditLog *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			et, action := inferEventType(r.Method, r.URL.Path)
			if et == "" {
				return // not an auditable endpoint
			}
			if rec.status() >= 500 {
				return // server errors — handler already logged
			}

			requestID := chimiddleware.GetReqID(r.Context())
			ip := realIP(r)

			auditLog.Record(&Event{
				EventType:  et,
				Action:     action,
				ActorType:  ActorAPIKey, // admin API is accessed via master key
				IPAddress:  ip,
				UserAgent:  r.UserAgent(),
				RequestID:  requestID,
				Metadata: map[string]any{
					"method":      r.Method,
					"path":        r.URL.Path,
					"status_code": rec.status(),
				},
			})
		})
	}
}

func inferEventType(method, path string) (eventType, action string) {
	for _, e := range auditableEndpoints {
		if e.method == method && strings.HasPrefix(path, e.prefix) {
			return e.eventType, e.action
		}
	}
	return "", ""
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.SplitN(forwarded, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
