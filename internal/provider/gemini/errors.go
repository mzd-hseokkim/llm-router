package gemini

import (
	"encoding/json"
	"net/http"

	"github.com/llm-router/gateway/internal/provider"
)

type geminiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"` // gRPC status name, e.g. "RESOURCE_EXHAUSTED"
	} `json:"error"`
}

// ParseError parses a Gemini error response and returns a normalized GatewayError.
// Gemini carries gRPC status names in the body, which override the HTTP status code.
func ParseError(status int, body []byte, header http.Header) *provider.GatewayError {
	msg, mapped := extractError(body, status)
	return provider.NormalizeHTTPError(mapped, msg, header)
}

func extractError(body []byte, httpStatus int) (msg string, status int) {
	var e geminiErrorBody
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Message == "" {
		return string(body), httpStatus
	}
	return e.Error.Message, mapGRPCStatus(e.Error.Status, httpStatus)
}

// mapGRPCStatus maps Gemini gRPC status names to standard HTTP codes.
func mapGRPCStatus(grpcStatus string, fallback int) int {
	switch grpcStatus {
	case "RESOURCE_EXHAUSTED":
		return 429
	case "UNAUTHENTICATED":
		return 401
	case "PERMISSION_DENIED":
		return 403
	case "NOT_FOUND":
		return 404
	case "INVALID_ARGUMENT", "FAILED_PRECONDITION":
		return 400
	case "UNAVAILABLE":
		return 503
	case "INTERNAL":
		return 500
	}
	return fallback
}
