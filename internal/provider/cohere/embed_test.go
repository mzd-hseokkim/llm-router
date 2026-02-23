package cohere

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

func TestCohereEmbed_StringNormalization(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": map[string]any{
				"float": [][]float64{{0.1, 0.2}},
			},
			"meta": map[string]any{
				"billed_units": map[string]int{"input_tokens": 1},
			},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "cohere/embed-english-v3.0",
		Input: json.RawMessage(`"hello"`),
	}
	resp, err := a.Embed(context.Background(), "embed-english-v3.0", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}

	// Verify string was normalized to []string in the request.
	texts, _ := capturedBody["texts"].([]any)
	if len(texts) != 1 {
		t.Errorf("expected texts to have 1 item, got %v", texts)
	}
	if texts[0] != "hello" {
		t.Errorf("expected texts[0]=%q, got %v", "hello", texts[0])
	}
}

func TestCohereEmbed_BatchInput(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": map[string]any{
				"float": [][]float64{{0.1}, {0.2}},
			},
			"meta": map[string]any{
				"billed_units": map[string]int{"input_tokens": 2},
			},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "cohere/embed-english-v3.0",
		Input: json.RawMessage(`["a", "b"]`),
	}
	resp, err := a.Embed(context.Background(), "embed-english-v3.0", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(resp.Data))
	}

	texts, _ := capturedBody["texts"].([]any)
	if len(texts) != 2 {
		t.Errorf("expected 2 texts, got %v", texts)
	}
}

func TestCohereEmbed_TokenParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": map[string]any{
				"float": [][]float64{{0.1}},
			},
			"meta": map[string]any{
				"billed_units": map[string]int{"input_tokens": 5},
			},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "cohere/embed-english-v3.0",
		Input: json.RawMessage(`"hello world"`),
	}
	resp, err := a.Embed(context.Background(), "embed-english-v3.0", req)
	if err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Errorf("expected TotalTokens=5, got %v", resp.Usage)
	}
}

func TestCohereEmbed_AuthHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"embeddings": map[string]any{"float": [][]float64{{0.1}}},
			"meta":       map[string]any{"billed_units": map[string]int{"input_tokens": 1}},
		})
	}))
	defer srv.Close()

	a := newTestAdapter(srv.URL)
	req := &types.EmbeddingRequest{
		Model: "cohere/embed-english-v3.0",
		Input: json.RawMessage(`"hello"`),
	}
	if _, err := a.Embed(context.Background(), "embed-english-v3.0", req); err != nil {
		t.Fatalf("Embed error: %v", err)
	}
	if !strings.HasPrefix(capturedAuth, "Bearer ") {
		t.Errorf("expected Bearer auth header, got %q", capturedAuth)
	}
}
