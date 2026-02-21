// Package selfhosted provides adapters for self-hosted LLM inference servers
// such as Ollama, vLLM, and Hugging Face TGI.
//
// All supported engines expose an OpenAI-compatible API, so this package
// delegates to the openai adapter with a custom base URL and no API key.
package selfhosted

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/provider/openai"
)

// Engine identifies the inference server implementation.
type Engine string

const (
	EngineOllama   Engine = "ollama"
	EngineVLLM     Engine = "vllm"
	EngineTGI      Engine = "tgi"
	EngineLMStudio Engine = "lmstudio"
)

// ModelEntry maps a Gateway model ID to the name used on the inference server.
type ModelEntry struct {
	ID        string // Gateway ID, e.g. "ollama/llama3.2:3b"
	ModelName string // Server-side name, e.g. "llama3.2:3b"
}

// Adapter wraps an openai.Adapter to talk to a self-hosted inference server.
type Adapter struct {
	inner   *openai.Adapter
	name    string
	engine  Engine
	baseURL string
	models  []ModelEntry
	hcURL   string // URL used for health-check GET
}

// New returns a self-hosted adapter.
// baseURL must include the scheme but no trailing slash (e.g. "http://localhost:11434").
func New(name, baseURL string, engine Engine, models []ModelEntry) *Adapter {
	// Determine the base URL for the OpenAI-compatible path.
	apiBase := openAIBase(baseURL, engine)

	// Self-hosted servers typically require no API key; pass an empty string.
	inner := openai.New("", apiBase)

	return &Adapter{
		inner:   inner,
		name:    name,
		engine:  engine,
		baseURL: baseURL,
		models:  models,
		hcURL:   healthCheckURL(baseURL, engine),
	}
}

// Name returns the provider name as registered in the Registry.
func (a *Adapter) Name() string { return a.name }

// Models returns the model list advertised by this instance.
func (a *Adapter) Models() []types.ModelInfo {
	infos := make([]types.ModelInfo, len(a.models))
	for i, m := range a.models {
		infos[i] = types.ModelInfo{ID: m.ID, Object: "model", OwnedBy: string(a.engine)}
	}
	return infos
}

// ChatCompletion sends a non-streaming request to the inference server.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (*types.ChatCompletionResponse, error) {
	serverModel := a.resolveModel(model)
	body, err := injectModel(rawBody, serverModel)
	if err != nil {
		return nil, fmt.Errorf("selfhosted %s: inject model: %w", a.name, err)
	}
	return a.inner.ChatCompletion(ctx, serverModel, req, body)
}

// ChatCompletionStream initiates a streaming request to the inference server.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (<-chan provider.StreamChunk, error) {
	serverModel := a.resolveModel(model)
	body, err := injectModel(rawBody, serverModel)
	if err != nil {
		return nil, fmt.Errorf("selfhosted %s: inject model: %w", a.name, err)
	}
	return a.inner.ChatCompletionStream(ctx, serverModel, req, body)
}

// HealthCheck verifies that the inference server is reachable and has at least
// one model loaded. Returns nil on success.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := http.NewRequestWithContext(ctx, http.MethodGet, a.hcURL, nil)
	if err != nil {
		return fmt.Errorf("selfhosted %s: build health request: %w", a.name, err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(resp)
	if err != nil {
		return provider.NewNetworkError(err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("selfhosted %s: health check HTTP %d: %s", a.name, res.StatusCode, string(b))
	}
	return nil
}

// resolveModel maps the Gateway model ID to the server-side model name.
// If no mapping is found, the part after the first "/" is used.
func (a *Adapter) resolveModel(gatewayID string) string {
	for _, m := range a.models {
		if m.ID == gatewayID {
			return m.ModelName
		}
	}
	// Fallback: strip "<provider>/" prefix.
	if idx := strings.Index(gatewayID, "/"); idx >= 0 {
		return gatewayID[idx+1:]
	}
	return gatewayID
}

// injectModel replaces the "model" field in a JSON body with serverModel.
func injectModel(rawBody []byte, serverModel string) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, err
	}
	modelJSON, _ := json.Marshal(serverModel)
	body["model"] = modelJSON
	return json.Marshal(body)
}

// openAIBase returns the base URL for the OpenAI-compatible API endpoint.
func openAIBase(baseURL string, engine Engine) string {
	switch engine {
	case EngineOllama:
		// Ollama exposes an OpenAI-compatible API at /v1.
		return strings.TrimRight(baseURL, "/") + "/v1"
	default:
		// vLLM, TGI, LMStudio all serve at /v1 already.
		return strings.TrimRight(baseURL, "/") + "/v1"
	}
}

// healthCheckURL returns a GET endpoint that can be used to verify liveness.
func healthCheckURL(baseURL string, engine Engine) string {
	base := strings.TrimRight(baseURL, "/")
	switch engine {
	case EngineOllama:
		return base + "/api/tags"
	default:
		// vLLM, TGI, LMStudio: /v1/models is standard.
		return base + "/v1/models"
	}
}
