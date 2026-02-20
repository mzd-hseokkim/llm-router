package gemini

import (
	"encoding/json"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func ptr[T any](v T) *T { return &v }

func TestBuildRequest_RoleMapping(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model: "google/gemini-2.0-flash",
		Messages: []types.Message{
			{Role: "system", Content: "Be helpful."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	}

	body, err := BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	var gr request
	if err := json.Unmarshal(body, &gr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// system goes to systemInstruction
	if gr.SystemInstruction == nil || gr.SystemInstruction.Parts[0].Text != "Be helpful." {
		t.Errorf("systemInstruction: got %+v", gr.SystemInstruction)
	}

	// 3 non-system messages
	if len(gr.Contents) != 3 {
		t.Fatalf("contents length: got %d, want 3", len(gr.Contents))
	}

	// assistant → "model"
	if gr.Contents[1].Role != "model" {
		t.Errorf("assistant role: got %q, want %q", gr.Contents[1].Role, "model")
	}
}

func TestBuildRequest_GenerationConfig(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model:       "google/gemini-2.0-flash",
		Messages:    []types.Message{{Role: "user", Content: "Hi"}},
		Temperature: ptr(0.5),
		MaxTokens:   ptr(512),
	}

	body, _ := BuildRequest(req)
	var gr request
	json.Unmarshal(body, &gr)

	if gr.GenerationConfig == nil {
		t.Fatal("generationConfig is nil")
	}
	if *gr.GenerationConfig.Temperature != 0.5 {
		t.Errorf("temperature: got %v", gr.GenerationConfig.Temperature)
	}
	if *gr.GenerationConfig.MaxOutputTokens != 512 {
		t.Errorf("maxOutputTokens: got %v", gr.GenerationConfig.MaxOutputTokens)
	}
}

func TestParseResponse_Mapping(t *testing.T) {
	respBody := []byte(`{
		"candidates": [{
			"content": {"parts": [{"text": "Hello!"}], "role": "model"},
			"finishReason": "STOP",
			"index": 0
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 6,
			"totalTokenCount": 16
		}
	}`)

	resp, err := ParseResponse("google/gemini-2.0-flash", respBody)
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}

	if resp.Model != "google/gemini-2.0-flash" {
		t.Errorf("model: got %q", resp.Model)
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("content: got %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason: got %q, want stop", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 6 || resp.Usage.TotalTokens != 16 {
		t.Errorf("usage: got %+v", resp.Usage)
	}
}

func TestParseResponse_NoCandidates(t *testing.T) {
	respBody := []byte(`{"candidates":[],"usageMetadata":{}}`)
	_, err := ParseResponse("google/gemini-2.0-flash", respBody)
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"STOP", "stop"},
		{"MAX_TOKENS", "length"},
		{"SAFETY", "content_filter"},
		{"OTHER", "stop"},
	}
	for _, tc := range tests {
		got := mapFinishReason(tc.input)
		if got != tc.want {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
