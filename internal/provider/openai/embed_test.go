package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func newTestAdapter(serverURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return "test-key", nil }, serverURL)
}

func TestEmbed_SingleString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "embedding": []float64{0.1, 0.2, 0.3}, "index": 0},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]int{"prompt_tokens": 3, "total_tokens": 3},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "openai/text-embedding-3-small",
		Input: json.RawMessage(`"hello world"`),
	}
	resp, err := a.Embed(context.Background(), "text-embedding-3-small", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}
	if len(resp.Data[0].Embedding) == 0 {
		t.Error("embedding should not be empty")
	}
	if resp.Model != "openai/text-embedding-3-small" {
		t.Errorf("model: got %q, want %q", resp.Model, "openai/text-embedding-3-small")
	}
}

func TestEmbed_BatchInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "embedding": []float64{0.1}, "index": 0},
				{"object": "embedding", "embedding": []float64{0.2}, "index": 1},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]int{"total_tokens": 4},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "openai/text-embedding-3-small",
		Input: json.RawMessage(`["a", "b"]`),
	}
	resp, err := a.Embed(context.Background(), "text-embedding-3-small", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(resp.Data))
	}
}

func TestEmbed_APIError429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "openai/text-embedding-3-small",
		Input: json.RawMessage(`"hello"`),
	}
	_, err := a.Embed(context.Background(), "text-embedding-3-small", req)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}

func TestEmbed_EmptyDataResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   []any{},
			"model":  "text-embedding-3-small",
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "openai/text-embedding-3-small",
		Input: json.RawMessage(`"hello"`),
	}
	resp, err := a.Embed(context.Background(), "text-embedding-3-small", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(resp.Data))
	}
}
