package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

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
	mock := &mockProvider{name: "mock"}
	h := NewCompletionsHandler(newTestRegistry(mock), discardLogger())

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
			name:       "unknown provider",
			body:       map[string]any{"model": "unknown/model", "prompt": "Hello"},
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
	mock := &mockProvider{name: "mock"}
	h := NewCompletionsHandler(newTestRegistry(mock), discardLogger())

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
}
