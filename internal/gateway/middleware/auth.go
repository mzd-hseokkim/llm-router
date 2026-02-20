package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// Auth validates the Authorization header on every /v1 request.
// This stub verifies that a Bearer token is present; full virtual key
// validation against the database is implemented in task 05.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(types.ErrorResponse{
				Error: types.ErrorDetail{
					Message: "invalid authentication: missing or malformed Authorization header",
					Type:    "invalid_request_error",
					Code:    "invalid_api_key",
				},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
