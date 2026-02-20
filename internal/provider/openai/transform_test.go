package openai

import (
	"encoding/json"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func TestBuildRequest_StripPrefix(t *testing.T) {
	rawBody := []byte(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}],"temperature":0.7}`)

	got, err := BuildRequest("gpt-4o", rawBody)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var model string
	if err := json.Unmarshal(body["model"], &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}
	if model != "gpt-4o" {
		t.Errorf("model: got %q, want %q", model, "gpt-4o")
	}
}

func TestBuildRequest_PreservesUnknownFields(t *testing.T) {
	// future or provider-specific fields should pass through unchanged
	rawBody := []byte(`{"model":"openai/gpt-4o","messages":[],"logprobs":true,"top_logprobs":5}`)

	got, err := BuildRequest("gpt-4o", rawBody)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var body map[string]json.RawMessage
	json.Unmarshal(got, &body)

	if _, ok := body["logprobs"]; !ok {
		t.Error("logprobs field should be preserved")
	}
	if _, ok := body["top_logprobs"]; !ok {
		t.Error("top_logprobs field should be preserved")
	}
}

func TestParseResponse_RestoresModel(t *testing.T) {
	respBody := []byte(`{
		"id": "chatcmpl-abc123",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "gpt-4o",
		"choices": [{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
	}`)

	resp, err := ParseResponse("openai/gpt-4o", respBody)
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}

	if resp.Model != "openai/gpt-4o" {
		t.Errorf("model: got %q, want %q", resp.Model, "openai/gpt-4o")
	}
	if resp.ID != "chatcmpl-abc123" {
		t.Errorf("id: got %q, want %q", resp.ID, "chatcmpl-abc123")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("content: got %q, want %q", resp.Choices[0].Message.Content, "Hello!")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Errorf("usage: got %v", resp.Usage)
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := ParseResponse("openai/gpt-4o", []byte("not-json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// Verify the adapter satisfies the provider.Provider interface at compile time.
var _ interface {
	Name() string
	Models() []types.ModelInfo
} = (*Adapter)(nil)
