package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/gateway/proxy"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// openAIStreamChunk is the wire format of a single OpenAI streaming chunk.
type openAIStreamChunk struct {
	ID      string                   `json:"id"`
	Choices []openAIStreamChunkChoice `json:"choices"`
	Usage   *types.Usage             `json:"usage"`
}

type openAIStreamChunkChoice struct {
	Delta        openAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type openAIStreamDelta struct {
	Content string `json:"content"`
}

// ChatCompletionStream opens a streaming connection to OpenAI and returns a
// channel that emits StreamChunks until the stream ends or ctx is cancelled.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve openai api key: %w", err)
	}

	body, err := buildStreamBody(model, rawBody)
	if err != nil {
		return nil, fmt.Errorf("build openai stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := newStreamHTTPClient().Do(httpReq)
	if err != nil {
		return nil, provider.NewUnavailableError(err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(b))
	}

	ch := make(chan provider.StreamChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		for event := range proxy.ParseSSE(resp.Body) {
			if event.Data == "[DONE]" {
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
				ch <- provider.StreamChunk{Error: fmt.Errorf("parse openai chunk: %w", err)}
				return
			}

			sc := provider.StreamChunk{Usage: chunk.Usage}
			if len(chunk.Choices) > 0 {
				sc.Delta = chunk.Choices[0].Delta.Content
				sc.FinishReason = chunk.Choices[0].FinishReason
			}
			ch <- sc
		}
	}()

	return ch, nil
}

// buildStreamBody clones rawBody, forces stream=true, and injects
// stream_options.include_usage so the final chunk carries token counts.
func buildStreamBody(model string, rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal openai stream body: %w", err)
	}
	modelJSON, _ := json.Marshal(model)
	body["model"] = modelJSON
	body["stream"] = json.RawMessage(`true`)
	body["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	return json.Marshal(body)
}

// newStreamHTTPClient returns an http.Client with no total request timeout,
// suitable for long-lived SSE streams. Cancellation is via context.
func newStreamHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   100,
		},
	}
}
