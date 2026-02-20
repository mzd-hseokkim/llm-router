package openai

import (
	"encoding/json"
	"fmt"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// BuildRequest takes the original rawBody and replaces the model field with
// the stripped model name (no "openai/" prefix). All other fields, including
// unknown parameters, are preserved as-is for pass-through.
func BuildRequest(model string, rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal openai request: %w", err)
	}
	modelJSON, _ := json.Marshal(model)
	body["model"] = json.RawMessage(modelJSON)
	return json.Marshal(body)
}

// ParseResponse decodes an OpenAI chat completions response and restores the
// original model name (with "openai/" prefix) in the returned struct.
func ParseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}
	resp.Model = originalModel
	return &resp, nil
}
