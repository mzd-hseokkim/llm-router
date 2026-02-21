package middleware

import (
	"net/http"

	"github.com/llm-router/gateway/internal/residency"
)

// DataResidency returns a middleware that reads the X-Data-Residency-Policy
// request header and stores it in the request context.
// The ChatHandler reads the policy name from context to filter fallback chains.
func DataResidency() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if policyName := r.Header.Get("X-Data-Residency-Policy"); policyName != "" {
				r = r.WithContext(residency.WithPolicy(r.Context(), policyName))
			}
			next.ServeHTTP(w, r)
		})
	}
}
