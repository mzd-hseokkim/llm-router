package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func postEmbeddings(t *testing.T, h *EmbeddingsHandler, body any) *httptest.ResponseRecorder {
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
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Handle(w, req)
	return w
}

func TestEmbeddingsHandler_StatusCodes(t *testing.T) {
	mock := &mockProvider{name: "mock"}
	h := NewEmbeddingsHandler(newTestRegistry(mock), discardLogger())

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "valid request",
			body:       map[string]any{"model": "mock/embed-model", "input": "Hello"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing model",
			body:       map[string]any{"input": "Hello"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing input",
			body:       map[string]any{"model": "mock/embed-model"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown provider",
			body:       map[string]any{"model": "unknown/model", "input": "Hello"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON",
			body:       "not-json{",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := postEmbeddings(t, h, tc.body)
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tc.wantStatus, w.Body)
			}
		})
	}
}

func TestEmbeddingsHandler_ResponseFormat(t *testing.T) {
	mock := &mockProvider{name: "mock"}
	h := NewEmbeddingsHandler(newTestRegistry(mock), discardLogger())

	w := postEmbeddings(t, h, map[string]any{"model": "mock/embed-model", "input": "Hello"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}

	var resp types.EmbeddingResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object: got %q, want %q", resp.Object, "list")
	}
	if resp.Data == nil {
		t.Error("data should not be nil")
	}
}
