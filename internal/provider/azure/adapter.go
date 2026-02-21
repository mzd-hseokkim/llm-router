// Package azure implements a provider adapter for Azure OpenAI Service.
// Azure uses the same request/response format as OpenAI, but the model is embedded
// in the URL as a deployment ID, and authentication uses the "api-key" header.
package azure

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

// Adapter implements provider.Provider for Azure OpenAI Service.
type Adapter struct {
	keyFunc     func(ctx context.Context) (string, error)
	resourceURL string // https://{resource}.openai.azure.com
	apiVersion  string
	client      *http.Client
	deployments []deployment
	mu          sync.RWMutex
	dbModels    []types.ModelInfo
}

type deployment struct {
	id    string // Azure deployment ID (used in URL)
	model string // logical model name (e.g. "gpt-4o")
}

// Config holds Azure OpenAI configuration.
type Config struct {
	ResourceName string
	APIKey       string
	APIVersion   string
	BaseURL      string // override for testing; normally derived from ResourceName
	Deployments  []DeploymentConfig
}

// DeploymentConfig maps an Azure deployment ID to a logical model name.
type DeploymentConfig struct {
	ID    string // Azure deployment ID
	Model string // logical model name
}

// New returns an Azure Adapter with a static API key.
func New(cfg Config) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) {
		return cfg.APIKey, nil
	}, cfg)
}

// NewManaged returns an Azure Adapter that resolves its API key from km at request time.
func NewManaged(km provider.KeyProvider, cfg Config) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "azure", "")
	}, cfg)
}

func newAdapter(keyFunc func(context.Context) (string, error), cfg Config) *Adapter {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s.openai.azure.com", cfg.ResourceName)
	}
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "2024-02-01"
	}

	deps := make([]deployment, 0, len(cfg.Deployments))
	for _, d := range cfg.Deployments {
		deps = append(deps, deployment{id: d.ID, model: d.Model})
	}

	return &Adapter{
		keyFunc:     keyFunc,
		resourceURL: baseURL,
		apiVersion:  apiVersion,
		client:      newHTTPClient(),
		deployments: deps,
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

func (a *Adapter) Name() string { return "azure" }

// SetModels injects a DB-sourced model list, overriding the deployment-derived default.
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
	models := make([]types.ModelInfo, 0, len(a.deployments))
	for _, d := range a.deployments {
		models = append(models, types.ModelInfo{
			ID:      "azure/" + d.id,
			Object:  "model",
			OwnedBy: "azure",
		})
	}
	return models
}

// ChatCompletion calls the Azure OpenAI chat completions endpoint.
// model is the deployment ID (the part after "azure/").
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve azure api key: %w", err)
	}

	body, err := BuildRequest(rawBody)
	if err != nil {
		return nil, fmt.Errorf("build azure request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		a.resourceURL, model, a.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create azure request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read azure response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(respBody), resp.Header)
	}

	return ParseResponse("azure/"+model, respBody)
}

// ChatCompletionStream initiates a streaming chat completion via Azure OpenAI.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve azure api key: %w", err)
	}

	body, err := BuildStreamRequest(rawBody)
	if err != nil {
		return nil, fmt.Errorf("build azure stream request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		a.resourceURL, model, a.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create azure stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", apiKey)
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

	return streamSSE(ctx, resp.Body, "azure/"+model), nil
}
