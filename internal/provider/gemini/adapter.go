package gemini

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

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Adapter implements provider.Provider for Google Gemini.
type Adapter struct {
	keyFunc func(ctx context.Context) (string, error)
	baseURL string
	client  *http.Client
}

// New returns a Gemini Adapter with a static API key.
// If baseURL is empty, the default is used.
func New(apiKey, baseURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return apiKey, nil }, baseURL)
}

// NewManaged returns a Gemini Adapter that resolves its API key from km at
// request time, enabling DB-backed key rotation without a server restart.
func NewManaged(km provider.KeyProvider, baseURL string) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "gemini", "")
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

func (a *Adapter) Name() string { return "google" }

func (a *Adapter) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "google/gemini-2.0-flash", Object: "model", OwnedBy: "google"},
		{ID: "google/gemini-2.0-pro", Object: "model", OwnedBy: "google"},
		{ID: "google/gemini-1.5-pro", Object: "model", OwnedBy: "google"},
		{ID: "google/gemini-1.5-flash", Object: "model", OwnedBy: "google"},
	}
}

// ChatCompletion converts the OpenAI request to Gemini format, calls the
// Gemini generateContent API, and converts the response back to OpenAI format.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve gemini api key: %w", err)
	}

	body, err := BuildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("build gemini request: %w", err)
	}

	// Gemini endpoint: /models/{model}:generateContent?key={apiKey}
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", a.baseURL, model, apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, ParseError(resp.StatusCode, respBody, resp.Header)
	}

	return ParseResponse(req.Model, respBody)
}
