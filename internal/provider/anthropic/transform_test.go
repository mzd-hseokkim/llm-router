package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func ptr[T any](v T) *T { return &v }

func TestBuildRequest_SystemExtracted(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "anthropic/claude-sonnet-4-20250514",
		Messages: []types.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}

	body, err := BuildRequest("claude-sonnet-4-20250514", req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var ar request
	if err := json.Unmarshal(body, &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if ar.System != "You are helpful." {
		t.Errorf("system: got %q, want %q", ar.System, "You are helpful.")
	}
	if len(ar.Messages) != 1 {
		t.Fatalf("messages length: got %d, want 1", len(ar.Messages))
	}
	if ar.Messages[0].Role != "user" {
		t.Errorf("messages[0].role: got %q, want %q", ar.Messages[0].Role, "user")
	}
}

func TestBuildRequest_DefaultMaxTokens(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model:    "anthropic/claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
	}

	body, err := BuildRequest("claude-sonnet-4-20250514", req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var ar request
	json.Unmarshal(body, &ar)

	if ar.MaxTokens != defaultMaxTokens {
		t.Errorf("max_tokens: got %d, want %d", ar.MaxTokens, defaultMaxTokens)
	}
}

func TestBuildRequest_ExplicitMaxTokens(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model:     "anthropic/claude-sonnet-4-20250514",
		Messages:  []types.Message{{Role: "user", Content: "Hi"}},
		MaxTokens: ptr(256),
	}

	body, _ := BuildRequest("claude-sonnet-4-20250514", req)
	var ar request
	json.Unmarshal(body, &ar)

	if ar.MaxTokens != 256 {
		t.Errorf("max_tokens: got %d, want 256", ar.MaxTokens)
	}
}

func TestBuildRequest_StopString(t *testing.T) {
	raw, _ := json.Marshal("END")
	req := &types.ChatCompletionRequest{
		Model:    "anthropic/claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
		Stop:     raw,
	}

	body, _ := BuildRequest("claude-sonnet-4-20250514", req)
	var ar request
	json.Unmarshal(body, &ar)

	if len(ar.StopSequences) != 1 || ar.StopSequences[0] != "END" {
		t.Errorf("stop_sequences: got %v, want [END]", ar.StopSequences)
	}
}

func TestBuildRequest_StopArray(t *testing.T) {
	raw, _ := json.Marshal([]string{"END", "DONE"})
	req := &types.ChatCompletionRequest{
		Model:    "anthropic/claude-sonnet-4-20250514",
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
		Stop:     raw,
	}

	body, _ := BuildRequest("claude-sonnet-4-20250514", req)
	var ar request
	json.Unmarshal(body, &ar)

	if len(ar.StopSequences) != 2 {
		t.Errorf("stop_sequences length: got %d, want 2", len(ar.StopSequences))
	}
}

func TestParseResponse_Mapping(t *testing.T) {
	respBody := []byte(`{
		"id": "msg_abc123",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-20250514",
		"content": [{"type": "text", "text": "Hello there!"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 12, "output_tokens": 8}
	}`)

	resp, err := ParseResponse("anthropic/claude-sonnet-4-20250514", respBody)
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}

	if resp.Model != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("model: got %q", resp.Model)
	}
	if resp.Choices[0].Message.Content != "Hello there!" {
		t.Errorf("content: got %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: got %q, want stop", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 12 || resp.Usage.CompletionTokens != 8 || resp.Usage.TotalTokens != 20 {
		t.Errorf("usage: got %+v", resp.Usage)
	}
}

func TestMapStopReason(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"end_turn", "stop"},
		{"stop_sequence", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
		{"unknown", "stop"},
	}
	for _, tc := range tests {
		got := mapStopReason(tc.input)
		if got != tc.want {
			t.Errorf("mapStopReason(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
