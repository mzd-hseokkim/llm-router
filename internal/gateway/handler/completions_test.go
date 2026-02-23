package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// streamMockProvider is a test double that returns configurable stream chunks.
type streamMockProvider struct {
	name   string
	chunks []provider.StreamChunk
}

func (s *streamMockProvider) Name() string              { return s.name }
func (s *streamMockProvider) Models() []types.ModelInfo { return nil }
func (s *streamMockProvider) ChatCompletion(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	return nil, nil
}
func (s *streamMockProvider) ChatCompletionStream(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(s.chunks))
	for _, c := range s.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// newCmplHandler creates a CompletionsHandler backed by the given providers.
func newCmplHandler(providers ...provider.Provider) *CompletionsHandler {
	reg := newTestRegistry(providers...)
	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	fr := fallback.NewRouter(reg, cb, discardLogger())
	return NewCompletionsHandler(fr, discardLogger())
}

func postCompletions(t *testing.T, h *CompletionsHandler, body any) *httptest.ResponseRecorder {
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
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handle(w, req)
	return w
}

func TestCompletionsHandler_StatusCodes(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		resp: &types.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "mock/text-model",
			Choices: []types.Choice{{Index: 0, Message: types.Message{Role: "assistant", Content: "hello"}, FinishReason: "stop"}},
		},
	}
	h := newCmplHandler(mock)

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "valid request",
			body:       map[string]any{"model": "mock/text-model", "prompt": "Hello"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing model",
			body:       map[string]any{"prompt": "Hello"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing prompt",
			body:       map[string]any{"model": "mock/text-model"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       "not-json{",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative max_tokens",
			body:       map[string]any{"model": "mock/text-model", "prompt": "Hello", "max_tokens": -1},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postCompletions(t, h, tc.body)
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tc.wantStatus, w.Body)
			}
		})
	}
}

func TestCompletionsHandler_ResponseFormat(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		resp: &types.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "mock/text-model",
			Choices: []types.Choice{{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: "world"},
				FinishReason: "stop",
			}},
		},
	}
	h := newCmplHandler(mock)

	w := postCompletions(t, h, map[string]any{"model": "mock/text-model", "prompt": "Hello"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}

	var resp types.CompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "text_completion" {
		t.Errorf("object: got %q, want %q", resp.Object, "text_completion")
	}
	if resp.Model != "mock/text-model" {
		t.Errorf("model: got %q, want %q", resp.Model, "mock/text-model")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices: got %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Text != "world" {
		t.Errorf("text: got %q, want %q", resp.Choices[0].Text, "world")
	}
}

func TestCompletionsHandler_ProviderError(t *testing.T) {
	// Use a non-retryable 400 error so the test doesn't wait on backoff delays.
	mock := &mockProvider{
		name: "mock",
		err: &provider.GatewayError{
			Code:       provider.ErrInvalidRequest,
			Message:    "bad request from provider",
			HTTPStatus: http.StatusBadRequest,
		},
	}
	h := newCmplHandler(mock)

	w := postCompletions(t, h, map[string]any{"model": "mock/text-model", "prompt": "Hello"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", w.Code, w.Body)
	}
}

func TestCompletionsHandler_Streaming(t *testing.T) {
	finish := "stop"
	mock := &streamMockProvider{
		name: "mock",
		chunks: []provider.StreamChunk{
			{Delta: "hello"},
			{Delta: " world", FinishReason: &finish},
		},
	}
	cb := circuitbreaker.New(circuitbreaker.DefaultConfig())
	reg := newTestRegistry(mock)
	fr := fallback.NewRouter(reg, cb, discardLogger())
	h := NewCompletionsHandler(fr, discardLogger())

	body, _ := json.Marshal(map[string]any{"model": "mock/text-model", "prompt": "Hello", "stream": true})
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Parse SSE chunks.
	scanner := bufio.NewScanner(w.Body)
	var chunks []types.CompletionStreamChunk
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk types.CompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("unmarshal chunk: %v", err)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one SSE chunk")
	}
	for _, c := range chunks {
		if c.Object != "text_completion" {
			t.Errorf("chunk object = %q, want %q", c.Object, "text_completion")
		}
	}

	// Verify text content is accumulated correctly.
	var got string
	for _, c := range chunks {
		if len(c.Choices) > 0 {
			got += c.Choices[0].Text
		}
	}
	if got != "hello world" {
		t.Errorf("accumulated text = %q, want %q", got, "hello world")
	}
}

// --- promptToMessages ---

func TestPromptToMessages(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string // expected Content of first message
		wantErr bool
	}{
		{name: "string prompt", input: `"Hello world"`, want: "Hello world"},
		{name: "array prompt single", input: `["Hello"]`, want: "Hello"},
		{name: "array prompt multi uses first", input: `["first","second"]`, want: "first"},
		{name: "empty string", input: `""`, wantErr: true},
		{name: "empty array", input: `[]`, wantErr: true},
		{name: "invalid json", input: `{bad}`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs, err := promptToMessages(json.RawMessage(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("len(msgs) = %d, want 1", len(msgs))
			}
			if msgs[0].Role != "user" {
				t.Errorf("role = %q, want %q", msgs[0].Role, "user")
			}
			if msgs[0].Content != tc.want {
				t.Errorf("content = %q, want %q", msgs[0].Content, tc.want)
			}
		})
	}
}
