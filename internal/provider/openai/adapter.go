package openai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Adapter implements provider.Provider for OpenAI.
type Adapter struct {
	keyFunc func(ctx context.Context) (string, error)
	baseURL string
	client  *http.Client
}

// New returns an OpenAI Adapter with a static API key.
// If baseURL is empty, the default is used.
func New(apiKey, baseURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return apiKey, nil }, baseURL)
}

// NewManaged returns an OpenAI Adapter that resolves its API key from km at
// request time, enabling DB-backed key rotation without a server restart.
func NewManaged(km provider.KeyProvider, baseURL string) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "openai", "")
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
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   100,
			ForceAttemptHTTP2:     true,
		},
	}
}

func (a *Adapter) Name() string { return "openai" }

func (a *Adapter) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "openai/gpt-4o", Object: "model", OwnedBy: "openai"},
		{ID: "openai/gpt-4o-mini", Object: "model", OwnedBy: "openai"},
		{ID: "openai/gpt-4-turbo", Object: "model", OwnedBy: "openai"},
		{ID: "openai/o1", Object: "model", OwnedBy: "openai"},
		{ID: "openai/o3-mini", Object: "model", OwnedBy: "openai"},
	}
}

// ChatCompletion forwards the request to OpenAI with the model prefix stripped.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve openai api key: %w", err)
	}

	body, err := BuildRequest(model, rawBody)
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create openai request: %w", err)
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
		return nil, fmt.Errorf("read openai response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ParseError(resp.StatusCode, respBody, resp.Header)
	}

	return ParseResponse(req.Model, respBody)
}
