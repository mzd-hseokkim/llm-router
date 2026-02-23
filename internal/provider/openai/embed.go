package openai

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

// Embed implements provider.EmbeddingProvider for OpenAI.
// model is the bare model name (without "openai/" prefix); req.Model retains the full name.
func (a *Adapter) Embed(ctx context.Context, model string, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve openai api key: %w", err)
	}

	modelJSON, _ := json.Marshal(model)
	reqBody := struct {
		Model          json.RawMessage `json:"model"`
		Input          json.RawMessage `json:"input"`
		EncodingFormat string          `json:"encoding_format,omitempty"`
	}{
		Model:          modelJSON,
		Input:          req.Input,
		EncodingFormat: req.EncodingFormat,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal openai embed request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai embed request: %w", err)
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
		return nil, fmt.Errorf("read openai embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ParseError(resp.StatusCode, respBody, resp.Header)
	}

	var result types.EmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal openai embed response: %w", err)
	}
	result.Model = req.Model // restore original model with provider prefix
	return &result, nil
}
