// Package mistral implements a provider adapter for Mistral AI.
// Mistral uses an OpenAI-compatible API format.
package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

const defaultBaseURL = "https://api.mistral.ai/v1"

// Adapter implements provider.Provider for Mistral AI.
type Adapter struct {
	keyFunc func(ctx context.Context) (string, error)
	baseURL string
	client  *http.Client
}

// New returns a Mistral Adapter with a static API key.
func New(apiKey, baseURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return apiKey, nil }, baseURL)
}

// NewManaged returns a Mistral Adapter that resolves its API key from km at request time.
func NewManaged(km provider.KeyProvider, baseURL string) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "mistral", "")
	}, baseURL)
}

func newAdapter(keyFunc func(context.Context) (string, error), baseURL string) *Adapter {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Adapter{
		keyFunc: keyFunc,
		baseURL: baseURL,
		client:  newHTTPClient(),
	}
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   100,
			ForceAttemptHTTP2:     true,
		},
	}
}

func (a *Adapter) Name() string { return "mistral" }

func (a *Adapter) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "mistral/mistral-large-latest", Object: "model", OwnedBy: "mistral"},
		{ID: "mistral/mistral-small-latest", Object: "model", OwnedBy: "mistral"},
		{ID: "mistral/codestral-latest", Object: "model", OwnedBy: "mistral"},
		{ID: "mistral/mistral-embed", Object: "model", OwnedBy: "mistral"},
	}
}

// ChatCompletion sends a chat completion request to Mistral.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve mistral api key: %w", err)
	}

	body, err := buildRequest(model, rawBody)
	if err != nil {
		return nil, fmt.Errorf("build mistral request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create mistral request: %w", err)
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
		return nil, fmt.Errorf("read mistral response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(respBody), resp.Header)
	}

	return parseResponse("mistral/"+model, respBody)
}

// ChatCompletionStream initiates a streaming request to Mistral.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve mistral api key: %w", err)
	}

	body, err := buildStreamRequest(model, rawBody)
	if err != nil {
		return nil, fmt.Errorf("build mistral stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create mistral stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(body), resp.Header)
	}

	return streamMistralSSE(ctx, resp.Body, "mistral/"+model), nil
}

// buildRequest replaces the model field with the stripped model name.
func buildRequest(model string, rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	modelJSON, _ := json.Marshal(model)
	body["model"] = json.RawMessage(modelJSON)
	return json.Marshal(body)
}

// buildStreamRequest sets stream=true in addition to updating the model field.
func buildStreamRequest(model string, rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	modelJSON, _ := json.Marshal(model)
	body["model"] = json.RawMessage(modelJSON)
	body["stream"] = json.RawMessage("true")
	return json.Marshal(body)
}

func parseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal mistral response: %w", err)
	}
	resp.Model = originalModel
	return &resp, nil
}

// streamMistralSSE reads Mistral's SSE stream (OpenAI-compatible format).
func streamMistralSSE(ctx context.Context, body io.ReadCloser, model string) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 16)
	go func() {
		defer close(ch)
		defer body.Close()

		buf := make([]byte, 4096)
		var leftover []byte

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := body.Read(buf)
			if n > 0 {
				leftover = append(leftover, buf[:n]...)
				leftover = parseMistralLines(ctx, ch, leftover)
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func parseMistralLines(ctx context.Context, ch chan<- provider.StreamChunk, buf []byte) []byte {
	for {
		idx := strings.IndexByte(string(buf), '\n')
		if idx < 0 {
			return buf
		}
		line := strings.TrimRight(string(buf[:idx]), "\r")
		buf = buf[idx+1:]

		if line == "" || line == "data: [DONE]" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := line[6:]
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *types.Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}

		sc := provider.StreamChunk{}
		if len(chunk.Choices) > 0 {
			sc.Delta = chunk.Choices[0].Delta.Content
			sc.FinishReason = chunk.Choices[0].FinishReason
		}
		sc.Usage = chunk.Usage

		select {
		case <-ctx.Done():
			return buf
		case ch <- sc:
		}
	}
}
