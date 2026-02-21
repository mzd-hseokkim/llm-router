package openai

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/llm-router/gateway/internal/provider"
)

type openaiErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"` // string or int depending on the error
	} `json:"error"`
}

// ParseError parses an OpenAI error response and returns a normalized GatewayError.
func ParseError(status int, body []byte, header http.Header) *provider.GatewayError {
	msg := extractMessage(body)
	return provider.NormalizeHTTPError(status, msg, header)
}

func extractMessage(body []byte) string {
	var e openaiErrorBody
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Message == "" {
		return string(body)
	}
	if e.Error.Code != nil {
		return fmt.Sprintf("%s (%v)", e.Error.Message, e.Error.Code)
	}
	return e.Error.Message
}
