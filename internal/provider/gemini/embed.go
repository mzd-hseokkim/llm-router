package gemini

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

// Embed implements provider.EmbeddingProvider for Gemini.
// model is the bare model name (e.g. "text-embedding-004"); req.Model retains the full name.
// Single string input uses embedContent; []string input uses batchEmbedContents.
func (a *Adapter) Embed(ctx context.Context, model string, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gemini api key: %w", err)
	}

	// Parse input: try single string first, then []string.
	var single string
	var batch []string
	if err := json.Unmarshal(req.Input, &single); err == nil {
		return a.embedSingle(ctx, apiKey, model, single, req.Model)
	}
	if err := json.Unmarshal(req.Input, &batch); err != nil {
		return nil, fmt.Errorf("parse gemini embed input: %w", err)
	}
	return a.embedBatch(ctx, apiKey, model, batch, req.Model)
}

func (a *Adapter) embedSingle(ctx context.Context, apiKey, model, text, originalModel string) (*types.EmbeddingResponse, error) {
	body, err := json.Marshal(map[string]any{
		"content": map[string]any{
			"parts": []map[string]string{{"text": text}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal gemini embed request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", a.baseURL, model, apiKey)
	respBody, err := a.doEmbedPost(ctx, url, body)
	if err != nil {
		return nil, err
	}

	var geminiResp struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini embed response: %w", err)
	}

	return &types.EmbeddingResponse{
		Object: "list",
		Data: []types.Embedding{{
			Object:    "embedding",
			Embedding: geminiResp.Embedding.Values,
			Index:     0,
		}},
		Model: originalModel,
	}, nil
}

func (a *Adapter) embedBatch(ctx context.Context, apiKey, model string, texts []string, originalModel string) (*types.EmbeddingResponse, error) {
	requests := make([]map[string]any, len(texts))
	for i, t := range texts {
		requests[i] = map[string]any{
			"content": map[string]any{
				"parts": []map[string]string{{"text": t}},
			},
		}
	}
	body, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		return nil, fmt.Errorf("marshal gemini batch embed request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", a.baseURL, model, apiKey)
	respBody, err := a.doEmbedPost(ctx, url, body)
	if err != nil {
		return nil, err
	}

	var batchResp struct {
		Embeddings []struct {
			Values []float64 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini batch embed response: %w", err)
	}

	data := make([]types.Embedding, len(batchResp.Embeddings))
	for i, e := range batchResp.Embeddings {
		data[i] = types.Embedding{
			Object:    "embedding",
			Embedding: e.Values,
			Index:     i,
		}
	}
	return &types.EmbeddingResponse{
		Object: "list",
		Data:   data,
		Model:  originalModel,
	}, nil
}

func (a *Adapter) doEmbedPost(ctx context.Context, url string, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create gemini embed request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ParseError(resp.StatusCode, respBody, resp.Header)
	}
	return respBody, nil
}
