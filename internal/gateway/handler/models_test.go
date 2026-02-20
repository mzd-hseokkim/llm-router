package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// providerWithModels is a mock provider that returns a fixed model list.
type providerWithModels struct {
	name   string
	models []types.ModelInfo
}

func (p *providerWithModels) Name() string              { return p.name }
func (p *providerWithModels) Models() []types.ModelInfo { return p.models }
func (p *providerWithModels) ChatCompletion(_ interface{ Done() <-chan struct{} }, _ string, _ *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	return nil, nil
}

// Use the full provider.Provider interface.
type fullMockProvider struct {
	mockProvider
	models []types.ModelInfo
}

func (m *fullMockProvider) Models() []types.ModelInfo { return m.models }

func newRegistryWithModels(models []types.ModelInfo) *provider.Registry {
	r := provider.NewRegistry()
	r.Register(&fullMockProvider{
		mockProvider: mockProvider{name: "mock"},
		models:       models,
	})
	return r
}

func TestModelsHandler_List_Empty(t *testing.T) {
	h := NewModelsHandler(provider.NewRegistry(), discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp types.ModelListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object: got %q, want %q", resp.Object, "list")
	}
	if resp.Data == nil {
		t.Error("data should not be nil")
	}
	if len(resp.Data) != 0 {
		t.Errorf("data length: got %d, want 0", len(resp.Data))
	}
}

func TestModelsHandler_List_WithModels(t *testing.T) {
	models := []types.ModelInfo{
		{ID: "mock/model-a", Object: "model", Created: time.Now().Unix(), OwnedBy: "mock"},
		{ID: "mock/model-b", Object: "model", Created: time.Now().Unix(), OwnedBy: "mock"},
	}
	h := NewModelsHandler(newRegistryWithModels(models), discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp types.ModelListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("data length: got %d, want 2", len(resp.Data))
	}
}

func TestModelsHandler_Get_Found(t *testing.T) {
	models := []types.ModelInfo{
		{ID: "mock/model-a", Object: "model", Created: 1234567890, OwnedBy: "mock"},
	}
	h := NewModelsHandler(newRegistryWithModels(models), discardLogger())

	// Use chi router to inject the wildcard URL param.
	r := chi.NewRouter()
	r.Get("/v1/models/*", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/mock/model-a", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body)
	}

	var m types.ModelInfo
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.ID != "mock/model-a" {
		t.Errorf("id: got %q, want %q", m.ID, "mock/model-a")
	}
}

func TestModelsHandler_Get_NotFound(t *testing.T) {
	h := NewModelsHandler(provider.NewRegistry(), discardLogger())

	r := chi.NewRouter()
	r.Get("/v1/models/*", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/unknown/model", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}

	var errResp types.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp.Error.Code != "model_not_found" {
		t.Errorf("error code: got %q, want %q", errResp.Error.Code, "model_not_found")
	}
}
