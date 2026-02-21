package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingProvider generates vector embeddings from text.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimensions() int
	ModelName() string
}

// openAIEmbedder calls the OpenAI Embeddings API to generate vectors.
type openAIEmbedder struct {
	apiKey     string
	model      string
	baseURL    string
	dimensions int
	client     *http.Client
}

// NewOpenAIEmbedder creates an embedder using the OpenAI embeddings API.
// model defaults to "text-embedding-3-small" (1536 dimensions).
func NewOpenAIEmbedder(apiKey, model, baseURL string) EmbeddingProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &openAIEmbedder{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		dimensions: 1536,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *openAIEmbedder) Dimensions() int    { return e.dimensions }
func (e *openAIEmbedder) ModelName() string  { return e.model }

func (e *openAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{
		"model": e.model,
		"input": text,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("embed marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("embed read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed api error %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embed unmarshal: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty embedding returned")
	}
	return result.Data[0].Embedding, nil
}
