package anthropic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1"
	anthropicVersion = "2023-06-01"
)

// Adapter implements provider.Provider for Anthropic Claude.
type Adapter struct {
	keyFunc  func(ctx context.Context) (string, error)
	baseURL  string
	client   *http.Client
	mu       sync.RWMutex
	dbModels []types.ModelInfo
}

// New returns an Anthropic Adapter with a static API key.
// If baseURL is empty, the default is used.
func New(apiKey, baseURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return apiKey, nil }, baseURL)
}

// NewManaged returns an Anthropic Adapter that resolves its API key from km at
// request time, enabling DB-backed key rotation without a server restart.
func NewManaged(km provider.KeyProvider, baseURL string) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "anthropic", "")
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

func (a *Adapter) Name() string { return "anthropic" }

// SetModels injects a DB-sourced model list, overriding the hardcoded default.
func (a *Adapter) SetModels(models []types.ModelInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dbModels = models
}

func (a *Adapter) Models() []types.ModelInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.dbModels != nil {
		return a.dbModels
	}
	return []types.ModelInfo{
		{ID: "anthropic/claude-sonnet-4-6", Object: "model", OwnedBy: "anthropic"},
		{ID: "anthropic/claude-haiku-4-5-20251001", Object: "model", OwnedBy: "anthropic"},
		// Legacy
		{ID: "anthropic/claude-opus-4-20250514", Object: "model", OwnedBy: "anthropic"},
		{ID: "anthropic/claude-sonnet-4-20250514", Object: "model", OwnedBy: "anthropic"},
	}
}

// ChatCompletion converts the OpenAI request to Anthropic format, calls the
// Anthropic messages API, and converts the response back to OpenAI format.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve anthropic api key: %w", err)
	}

	body, err := BuildRequest(model, req)
	if err != nil {
		return nil, fmt.Errorf("build anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ParseError(resp.StatusCode, respBody, resp.Header)
	}

	return ParseResponse(req.Model, respBody)
}
