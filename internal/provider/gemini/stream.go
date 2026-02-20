package gemini

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

// ChatCompletionStream opens a streaming connection to Gemini's
// streamGenerateContent endpoint and returns a channel of StreamChunks.
//
// Gemini returns cumulative text in each event, so we compute the delta by
// subtracting the previously seen accumulated text.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gemini api key: %w", err)
	}

	body, err := BuildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build gemini stream request: %w", err)
	}

	// Gemini streaming endpoint uses streamGenerateContent with alt=sse.
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse",
		a.baseURL, model, apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create gemini stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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

		var accumulated string // Gemini sends cumulative text; track for delta calc.

		for event := range proxy.ParseSSE(resp.Body) {
			var gr response
			if err := json.Unmarshal([]byte(event.Data), &gr); err != nil {
				ch <- provider.StreamChunk{Error: fmt.Errorf("parse gemini stream event: %w", err)}
				return
			}

			if len(gr.Candidates) == 0 {
				continue
			}

			cand := gr.Candidates[0]

			// Concatenate all parts to get the cumulative text for this event.
			cumulative := ""
			for _, p := range cand.Content.Parts {
				cumulative += p.Text
			}

			// Compute delta from previous accumulated text.
			delta := ""
			if len(cumulative) > len(accumulated) {
				delta = cumulative[len(accumulated):]
				accumulated = cumulative
			}

			if cand.FinishReason != "" && cand.FinishReason != "FINISH_REASON_UNSPECIFIED" {
				reason := mapFinishReason(cand.FinishReason)
				var usage *types.Usage
				if gr.UsageMetadata.TotalTokenCount > 0 {
					usage = &types.Usage{
						PromptTokens:     gr.UsageMetadata.PromptTokenCount,
						CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
						TotalTokens:      gr.UsageMetadata.TotalTokenCount,
					}
				}
				if delta != "" {
					ch <- provider.StreamChunk{Delta: delta}
				}
				ch <- provider.StreamChunk{FinishReason: &reason, Usage: usage}
				return
			}

			if delta != "" {
				ch <- provider.StreamChunk{Delta: delta}
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
