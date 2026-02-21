package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// Recovery returns a middleware that catches panics, logs them with a stack trace,
// and returns a 500 Internal Server Error in OpenAI-compatible JSON format.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"panic", rec,
						"path", r.URL.Path,
						"stack", string(debug.Stack()),
					)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(types.ErrorResponse{
						Error: types.ErrorDetail{
							Message: "internal server error",
							Type:    "internal_error",
						},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
