package middleware

import (
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const gatewayVersion = "1.0.0"

// RequestMeta injects gateway-specific headers into every response.
// It relies on chi's RequestID middleware having already set the request ID
// in the context (applied in server.New).
func RequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", chimiddleware.GetReqID(r.Context()))
		w.Header().Set("X-Gateway-Version", gatewayVersion)
		next.ServeHTTP(w, r)
	})
}
