package anthropic

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

// --- Anthropic streaming event wire types ---

type anthropicStreamEvent struct {
	Type    string                `json:"type"`
	Index   int                   `json:"index"`
	Delta   json.RawMessage       `json:"delta"`
	Message *anthropicMsg         `json:"message"` // message_start
	Usage   *anthropicStreamUsage `json:"usage"`   // message_delta usage
}

type anthropicMsg struct {
	Usage anthropicMsgUsage `json:"usage"`
}

type anthropicMsgUsage struct {
	InputTokens int `json:"input_tokens"`
}

type anthropicStreamUsage struct {
	OutputTokens int `json:"output_tokens"`
}

type anthropicTextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicStopDelta struct {
	StopReason string `json:"stop_reason"`
}

// ChatCompletionStream opens a streaming connection to Anthropic and returns a
// channel of StreamChunks. The stream is cancelled when ctx is done.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve anthropic api key: %w", err)
	}

	// Force stream=true in the request body.
	streamReq := *req
	streamReq.Stream = true

	body, err := BuildRequest(model, &streamReq)
	if err != nil {
		return nil, fmt.Errorf("build anthropic stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

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

		var inputTokens int

		for event := range proxy.ParseSSE(resp.Body) {
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(event.Data), &ev); err != nil {
				ch <- provider.StreamChunk{Error: fmt.Errorf("parse anthropic event: %w", err)}
				return
			}

			switch ev.Type {
			case "message_start":
				if ev.Message != nil {
					inputTokens = ev.Message.Usage.InputTokens
				}

			case "content_block_delta":
				var d anthropicTextDelta
				if err := json.Unmarshal(ev.Delta, &d); err != nil {
					continue
				}
				if d.Type == "text_delta" && d.Text != "" {
					ch <- provider.StreamChunk{Delta: d.Text}
				}

			case "message_delta":
				// Carries finish reason and output token count.
				var d anthropicStopDelta
				if err := json.Unmarshal(ev.Delta, &d); err != nil {
					continue
				}
				reason := mapStopReason(d.StopReason)

				var usage *types.Usage
				if ev.Usage != nil {
					usage = &types.Usage{
						PromptTokens:     inputTokens,
						CompletionTokens: ev.Usage.OutputTokens,
						TotalTokens:      inputTokens + ev.Usage.OutputTokens,
					}
				}
				ch <- provider.StreamChunk{FinishReason: &reason, Usage: usage}

			case "message_stop":
				return
			}
		}
	}()

	return ch, nil
}

// newStreamHTTPClient returns an http.Client with no total timeout.
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
