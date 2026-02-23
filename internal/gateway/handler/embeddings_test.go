package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// mockEmbeddingProvider extends mockProvider with EmbeddingProvider support.
type mockEmbeddingProvider struct {
	mockProvider
	embedResp *types.EmbeddingResponse
	embedErr  error
}

func (m *mockEmbeddingProvider) Embed(_ context.Context, _ string, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	if m.embedErr != nil {
		return nil, m.embedErr
	}
	if m.embedResp != nil {
		return m.embedResp, nil
	}
	return &types.EmbeddingResponse{
		Object: "list",
		Data:   []types.Embedding{{Object: "embedding", Embedding: []float64{0.1, 0.2}, Index: 0}},
		Model:  req.Model,
	}, nil
}

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
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "valid request — provider has no EmbeddingProvider",
			body:       map[string]any{"model": "mock/embed-model", "input": "Hello"},
			wantStatus: http.StatusBadRequest, // mock doesn't implement EmbeddingProvider
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

func TestEmbeddingsHandler_SuccessWithEmbeddingProvider(t *testing.T) {
	mock := &mockEmbeddingProvider{mockProvider: mockProvider{name: "mock"}}
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

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
	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		t.Error("expected non-empty embedding data")
	}
}

func TestEmbeddingsHandler_UnsupportedProvider(t *testing.T) {
	// mockProvider does NOT implement EmbeddingProvider.
	mock := &mockProvider{name: "mock"}
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

	w := postEmbeddings(t, h, map[string]any{"model": "mock/embed", "input": "Hello"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEmbeddingsHandler_ProviderError(t *testing.T) {
	mock := &mockEmbeddingProvider{
		mockProvider: mockProvider{name: "mock"},
		embedErr:     &provider.GatewayError{Code: provider.ErrProviderError, Message: "upstream error", HTTPStatus: 502},
	}
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

	w := postEmbeddings(t, h, map[string]any{"model": "mock/embed", "input": "Hello"})
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestEmbeddingsHandler_NilCacheWorks(t *testing.T) {
	mock := &mockEmbeddingProvider{mockProvider: mockProvider{name: "mock"}}
	// nil cache and costCalc — should work fine.
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

	w := postEmbeddings(t, h, map[string]any{"model": "mock/embed", "input": "Hello"})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body = %s", w.Code, w.Body)
	}
}

func TestEmbeddingsHandler_ResponseFormat(t *testing.T) {
	mock := &mockEmbeddingProvider{mockProvider: mockProvider{name: "mock"}}
	h := NewEmbeddingsHandler(newTestRegistry(mock), nil, nil, discardLogger())

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
