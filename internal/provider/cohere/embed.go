package cohere

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

type cohereEmbedRequest struct {
	Model          string   `json:"model"`
	Texts          []string `json:"texts"`
	InputType      string   `json:"input_type"`
	EmbeddingTypes []string `json:"embedding_types"`
}

type cohereEmbedResponse struct {
	Embeddings struct {
		Float [][]float64 `json:"float"`
	} `json:"embeddings"`
	Meta struct {
		BilledUnits struct {
			InputTokens int `json:"input_tokens"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// Embed implements provider.EmbeddingProvider for Cohere.
// model is the bare model name (e.g. "embed-english-v3.0"); req.Model retains the full name.
func (a *Adapter) Embed(ctx context.Context, model string, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve cohere api key: %w", err)
	}

	// Normalize input to []string.
	var texts []string
	var single string
	if err := json.Unmarshal(req.Input, &single); err == nil {
		texts = []string{single}
	} else if err := json.Unmarshal(req.Input, &texts); err != nil {
		return nil, fmt.Errorf("parse cohere embed input: %w", err)
	}

	cohReq := cohereEmbedRequest{
		Model:          model,
		Texts:          texts,
		InputType:      "search_document",
		EmbeddingTypes: []string{"float"},
	}
	body, err := json.Marshal(cohReq)
	if err != nil {
		return nil, fmt.Errorf("marshal cohere embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cohere embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cohere embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(respBody), resp.Header)
	}

	var cr cohereEmbedResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return nil, fmt.Errorf("unmarshal cohere embed response: %w", err)
	}

	data := make([]types.Embedding, len(cr.Embeddings.Float))
	for i, v := range cr.Embeddings.Float {
		data[i] = types.Embedding{
			Object:    "embedding",
			Embedding: v,
			Index:     i,
		}
	}

	var usage *types.Usage
	if n := cr.Meta.BilledUnits.InputTokens; n > 0 {
		usage = &types.Usage{TotalTokens: n}
	}

	return &types.EmbeddingResponse{
		Object: "list",
		Data:   data,
		Model:  req.Model,
		Usage:  usage,
	}, nil
}
