package handler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// writeError writes an OpenAI-compatible error response.
func writeError(w http.ResponseWriter, status int, message, errType, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// generateID returns a unique ID with the given prefix (e.g. "chatcmpl-<hex>").
func generateID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-fallback", prefix)
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}
