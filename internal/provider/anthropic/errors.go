package anthropic

import (
	"encoding/json"
	"net/http"

	"github.com/llm-router/gateway/internal/provider"
)

type anthropicErrorBody struct {
	Type  string `json:"type"` // "error"
	Error struct {
		Type    string `json:"type"`    // "overloaded_error", "authentication_error", etc.
		Message string `json:"message"`
	} `json:"error"`
}

// ParseError parses an Anthropic error response and returns a normalized GatewayError.
// Anthropic uses HTTP 529 for overloaded, which is mapped to 503.
func ParseError(status int, body []byte, header http.Header) *provider.GatewayError {
	msg := extractMessage(body)
	// Anthropic uses non-standard 529 for overloaded; treat as 503.
	if status == 529 {
		status = 503
	}
	return provider.NormalizeHTTPError(status, msg, header)
}

func extractMessage(body []byte) string {
	var e anthropicErrorBody
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Message == "" {
		return string(body)
	}
	return e.Error.Message
}
