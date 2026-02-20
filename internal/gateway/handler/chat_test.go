package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// mockProvider is a test double for provider.Provider.
type mockProvider struct {
	name string
	resp *types.ChatCompletionResponse
	err  error
}

func (m *mockProvider) Name() string              { return m.name }
func (m *mockProvider) Models() []types.ModelInfo { return nil }
func (m *mockProvider) ChatCompletion(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	return m.resp, m.err
}
func (m *mockProvider) ChatCompletionStream(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk)
	close(ch)
	return ch, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestRegistry(providers ...provider.Provider) *provider.Registry {
	r := provider.NewRegistry()
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

// --- parseModel ---

func TestParseModel(t *testing.T) {
	tests := []struct {
		input    string
		provider string
		model    string
	}{
		{"anthropic/claude-sonnet-4-20250514", "anthropic", "claude-sonnet-4-20250514"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"google/gemini-2.0-flash", "google", "gemini-2.0-flash"},
		{"gpt-4o", "openai", "gpt-4o"},                   // no prefix → default openai
		{"/gpt-4o", "openai", "/gpt-4o"},                  // empty provider → default openai
		{"a/b/c", "a", "b/c"},                             // extra slash stays in model
		{"anthropic/claude-3-5-haiku-20241022", "anthropic", "claude-3-5-haiku-20241022"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := parseModel(tc.input)
			if got.Provider != tc.provider {
				t.Errorf("provider: got %q, want %q", got.Provider, tc.provider)
			}
			if got.Model != tc.model {
				t.Errorf("model: got %q, want %q", got.Model, tc.model)
			}
		})
	}
}

// --- validateChatRequest ---

func TestValidateChatRequest(t *testing.T) {
	ptr := func(f float64) *float64 { return &f }
	iptr := func(i int) *int { return &i }

	tests := []struct {
		name    string
		req     *types.ChatCompletionRequest
		wantErr bool
	}{
		{
			name: "valid minimal",
			req: &types.ChatCompletionRequest{
				Model:    "anthropic/claude-sonnet-4",
				Messages: []types.Message{{Role: "user", Content: "hi"}},
			},
		},
		{
			name:    "missing model",
			req:     &types.ChatCompletionRequest{Messages: []types.Message{{Role: "user", Content: "hi"}}},
			wantErr: true,
		},
		{
			name:    "missing messages",
			req:     &types.ChatCompletionRequest{Model: "anthropic/claude-sonnet-4"},
			wantErr: true,
		},
		{
			name: "empty messages slice",
			req: &types.ChatCompletionRequest{
				Model:    "anthropic/claude-sonnet-4",
				Messages: []types.Message{},
			},
			wantErr: true,
		},
		{
			name: "temperature exactly 0",
			req: &types.ChatCompletionRequest{
				Model:       "anthropic/claude-sonnet-4",
				Messages:    []types.Message{{Role: "user", Content: "hi"}},
				Temperature: ptr(0.0),
			},
		},
		{
			name: "temperature exactly 2",
			req: &types.ChatCompletionRequest{
				Model:       "anthropic/claude-sonnet-4",
				Messages:    []types.Message{{Role: "user", Content: "hi"}},
				Temperature: ptr(2.0),
			},
		},
		{
			name: "temperature above 2",
			req: &types.ChatCompletionRequest{
				Model:       "anthropic/claude-sonnet-4",
				Messages:    []types.Message{{Role: "user", Content: "hi"}},
				Temperature: ptr(2.1),
			},
			wantErr: true,
		},
		{
			name: "negative temperature",
			req: &types.ChatCompletionRequest{
				Model:       "anthropic/claude-sonnet-4",
				Messages:    []types.Message{{Role: "user", Content: "hi"}},
				Temperature: ptr(-0.1),
			},
			wantErr: true,
		},
		{
			name: "positive max_tokens",
			req: &types.ChatCompletionRequest{
				Model:     "anthropic/claude-sonnet-4",
				Messages:  []types.Message{{Role: "user", Content: "hi"}},
				MaxTokens: iptr(100),
			},
		},
		{
			name: "zero max_tokens",
			req: &types.ChatCompletionRequest{
				Model:     "anthropic/claude-sonnet-4",
				Messages:  []types.Message{{Role: "user", Content: "hi"}},
				MaxTokens: iptr(0),
			},
			wantErr: true,
		},
		{
			name: "negative max_tokens",
			req: &types.ChatCompletionRequest{
				Model:     "anthropic/claude-sonnet-4",
				Messages:  []types.Message{{Role: "user", Content: "hi"}},
				MaxTokens: iptr(-1),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChatRequest(tc.req)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateChatRequest() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// --- ChatHandler.Handle ---

func mockResponse(model string) *types.ChatCompletionResponse {
	return &types.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   model,
		Choices: []types.Choice{{
			Index:        0,
			Message:      types.Message{Role: "assistant", Content: "Hello!"},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func postJSON(t *testing.T, h *ChatHandler, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	switch v := body.(type) {
	case string:
		b = []byte(v)
	default:
		var err error
		b, err = json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handle(w, req)
	return w
}

func TestChatHandler_StatusCodes(t *testing.T) {
	mock := &mockProvider{name: "mock", resp: mockResponse("mock/model")}
	h := NewChatHandler(newTestRegistry(mock), discardLogger())

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name: "valid request",
			body: map[string]any{
				"model":    "mock/model",
				"messages": []map[string]string{{"role": "user", "content": "Hello"}},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing model",
			body:       map[string]any{"messages": []map[string]string{{"role": "user", "content": "Hello"}}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing messages",
			body:       map[string]any{"model": "mock/model"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       "not-json{",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown provider",
			body: map[string]any{
				"model":    "unknown/model",
				"messages": []map[string]string{{"role": "user", "content": "Hello"}},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "temperature out of range",
			body: map[string]any{
				"model":       "mock/model",
				"messages":    []map[string]string{{"role": "user", "content": "Hello"}},
				"temperature": 3.0,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(t, h, tc.body)
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tc.wantStatus, w.Body)
			}
		})
	}
}

func TestChatHandler_ResponseBody(t *testing.T) {
	expected := mockResponse("mock/model")
	mock := &mockProvider{name: "mock", resp: expected}
	h := NewChatHandler(newTestRegistry(mock), discardLogger())

	w := postJSON(t, h, map[string]any{
		"model":    "mock/model",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}

	var got types.ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != expected.ID {
		t.Errorf("ID: got %q, want %q", got.ID, expected.ID)
	}
	if got.Object != expected.Object {
		t.Errorf("Object: got %q, want %q", got.Object, expected.Object)
	}
	if len(got.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(got.Choices))
	}
	if got.Choices[0].Message.Content != "Hello!" {
		t.Errorf("content: got %q, want %q", got.Choices[0].Message.Content, "Hello!")
	}
	if got.Usage == nil || got.Usage.TotalTokens != 15 {
		t.Errorf("usage total_tokens: got %v", got.Usage)
	}
}

func TestChatHandler_ErrorResponseFormat(t *testing.T) {
	h := NewChatHandler(newTestRegistry(), discardLogger())

	// missing model triggers a 400 with OpenAI error format
	w := postJSON(t, h, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var errResp types.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("error.type: got %q, want %q", errResp.Error.Type, "invalid_request_error")
	}
	if errResp.Error.Message == "" {
		t.Error("error.message must not be empty")
	}
}

func TestChatHandler_ContentTypeJSON(t *testing.T) {
	mock := &mockProvider{name: "mock", resp: mockResponse("mock/model")}
	h := NewChatHandler(newTestRegistry(mock), discardLogger())

	w := postJSON(t, h, map[string]any{
		"model":    "mock/model",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	})

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

func TestChatHandler_ProviderError(t *testing.T) {
	mock := &mockProvider{name: "mock", err: fmt.Errorf("upstream timeout")}
	h := NewChatHandler(newTestRegistry(mock), discardLogger())

	w := postJSON(t, h, map[string]any{
		"model":    "mock/model",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	})

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body = %s", w.Code, w.Body)
	}

	var errResp types.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Type != "api_error" {
		t.Errorf("error.type: got %q, want %q", errResp.Error.Type, "api_error")
	}
}

func TestChatHandler_Streaming(t *testing.T) {
	mock := &mockProvider{name: "mock", resp: mockResponse("mock/model")}
	h := NewChatHandler(newTestRegistry(mock), discardLogger())

	body, _ := json.Marshal(map[string]any{
		"model":    "mock/model",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
		"stream":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream; charset=utf-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/event-stream; charset=utf-8")
	}

	// SSE body must contain data: lines and [DONE]
	bodyStr := w.Body.String()
	if len(bodyStr) == 0 {
		t.Fatal("streaming body is empty")
	}
	if !contains(bodyStr, "data:") {
		t.Error("streaming body must contain SSE data lines")
	}
	if !contains(bodyStr, "[DONE]") {
		t.Error("streaming body must end with [DONE]")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
