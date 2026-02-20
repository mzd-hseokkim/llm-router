package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// fakeProvider is a minimal provider.Provider implementation for streaming tests.
type fakeProvider struct {
	chunks []provider.StreamChunk
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Models() []types.ModelInfo { return nil }
func (f *fakeProvider) ChatCompletion(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	return nil, nil
}
func (f *fakeProvider) ChatCompletionStream(_ context.Context, _ string, _ *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func TestStreamToClient_NormalFlow(t *testing.T) {
	stop := "stop"
	fp := &fakeProvider{chunks: []provider.StreamChunk{
		{Delta: "Hello"},
		{Delta: " world"},
		{FinishReason: &stop},
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	parsed := types.ParsedModel{Provider: "fake", Model: "fake-model"}
	chatReq := &types.ChatCompletionRequest{Model: "fake/fake-model", Messages: []types.Message{{Role: "user", Content: "hi"}}}

	StreamToClient(w, req, fp, parsed, chatReq, nil, testLogger())

	body := w.Body.String()

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("unexpected Content-Type: %s", ct)
	}
	if !strings.Contains(body, "data: ") {
		t.Error("expected SSE data lines")
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Error("expected [DONE] terminator")
	}
	if !strings.Contains(body, `"role":"assistant"`) {
		t.Error("expected role chunk")
	}
	if !strings.Contains(body, `"Hello"`) {
		t.Error("expected Hello content")
	}
	if !strings.Contains(body, `"stop"`) {
		t.Error("expected stop finish_reason")
	}
}

func TestStreamToClient_WithUsage(t *testing.T) {
	stop := "stop"
	fp := &fakeProvider{chunks: []provider.StreamChunk{
		{Delta: "hi"},
		{FinishReason: &stop, Usage: &types.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}},
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	parsed := types.ParsedModel{Provider: "fake", Model: "fake-model"}
	chatReq := &types.ChatCompletionRequest{Model: "fake/fake-model", Messages: []types.Message{{Role: "user", Content: "hi"}}}

	StreamToClient(w, req, fp, parsed, chatReq, nil, testLogger())

	body := w.Body.String()
	if !strings.Contains(body, `"total_tokens":8`) {
		t.Errorf("expected usage in body, got:\n%s", body)
	}
}

func TestStreamToClient_ProviderError(t *testing.T) {
	fp := &fakeProvider{chunks: []provider.StreamChunk{
		{Error: errFake("provider exploded")},
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	parsed := types.ParsedModel{Provider: "fake", Model: "fake-model"}
	chatReq := &types.ChatCompletionRequest{Model: "fake/fake-model", Messages: []types.Message{{Role: "user", Content: "hi"}}}

	StreamToClient(w, req, fp, parsed, chatReq, nil, testLogger())

	body := w.Body.String()
	if !strings.Contains(body, "provider exploded") {
		t.Errorf("expected error message in body, got:\n%s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Error("expected [DONE] after error")
	}
}

func TestBuildChunk_DeltaChunk(t *testing.T) {
	sc := provider.StreamChunk{Delta: "hello"}
	c := buildChunk("id1", 1000, "test-model", sc)

	if c.Choices[0].Delta.Content != "hello" {
		t.Errorf("unexpected delta: %q", c.Choices[0].Delta.Content)
	}
	if c.Choices[0].FinishReason != "" {
		t.Error("finish_reason should be empty for non-finish chunk")
	}
}

func TestBuildChunk_FinishChunk(t *testing.T) {
	stop := "stop"
	sc := provider.StreamChunk{FinishReason: &stop}
	c := buildChunk("id1", 1000, "test-model", sc)

	if c.Choices[0].FinishReason != "stop" {
		t.Errorf("unexpected finish_reason: %q", c.Choices[0].FinishReason)
	}
	if c.Choices[0].Delta.Content != "" {
		t.Error("delta should be empty for finish chunk")
	}
}

func TestWriteChunk_ValidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	chunk := types.ChatCompletionChunk{
		ID:     "test-id",
		Object: "chat.completion.chunk",
		Model:  "test-model",
	}
	writeChunk(w, chunk)

	line := w.Body.String()
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected SSE data line, got: %s", line)
	}
	jsonPart := strings.TrimPrefix(line, "data: ")
	jsonPart = strings.TrimRight(jsonPart, "\n")
	var out types.ChatCompletionChunk
	if err := json.Unmarshal([]byte(jsonPart), &out); err != nil {
		t.Fatalf("invalid JSON in SSE line: %v\nline: %s", err, line)
	}
	if out.ID != "test-id" {
		t.Errorf("unexpected ID: %q", out.ID)
	}
}

// --- helpers ---

type errFake string

func (e errFake) Error() string { return string(e) }

func testLogger() *slog.Logger {
	return slog.Default()
}
