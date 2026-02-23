package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func newTestAdapter(serverURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return "test-key", nil }, serverURL)
}

func TestGeminiEmbed_SingleString(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{
				"values": []float64{0.1, 0.2, 0.3},
			},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "google/text-embedding-004",
		Input: json.RawMessage(`"hello"`),
	}
	resp, err := a.Embed(context.Background(), "text-embedding-004", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}
	if len(resp.Data[0].Embedding) != 3 {
		t.Errorf("expected 3 values, got %d", len(resp.Data[0].Embedding))
	}
	if !strings.Contains(capturedURL, "embedContent") {
		t.Errorf("expected embedContent endpoint, got %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "key=test-key") {
		t.Errorf("expected API key in URL, got %s", capturedURL)
	}
}

func TestGeminiEmbed_BatchInput(t *testing.T) {
	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": []map[string]any{
				{"values": []float64{0.1}},
				{"values": []float64{0.2}},
				{"values": []float64{0.3}},
			},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "google/text-embedding-004",
		Input: json.RawMessage(`["a", "b", "c"]`),
	}
	resp, err := a.Embed(context.Background(), "text-embedding-004", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Errorf("expected 3 embeddings, got %d", len(resp.Data))
	}
	if !strings.Contains(capturedURL, "batchEmbedContents") {
		t.Errorf("expected batchEmbedContents endpoint, got %s", capturedURL)
	}
}

func TestGeminiEmbed_APIError400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":400,"message":"invalid model","status":"INVALID_ARGUMENT"}}`))
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "google/text-embedding-004",
		Input: json.RawMessage(`"hello"`),
	}
	_, err := a.Embed(context.Background(), "text-embedding-004", req)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}
