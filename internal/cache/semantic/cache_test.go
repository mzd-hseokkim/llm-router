package semantic

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

type mockEmbedder struct {
	vec []float32
	err error
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.vec, m.err
}
func (m *mockEmbedder) Dimensions() int   { return len(m.vec) }
func (m *mockEmbedder) ModelName() string { return "mock" }

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBuildCacheText(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		messages []types.Message
		want     string
	}{
		{
			name:     "single user message",
			model:    "openai/gpt-4o",
			messages: []types.Message{{Role: "user", Content: "hello world"}},
			want:     "[model:openai/gpt-4o] hello world",
		},
		{
			name:  "last user message used",
			model: "openai/gpt-4o",
			messages: []types.Message{
				{Role: "user", Content: "first"},
				{Role: "assistant", Content: "reply"},
				{Role: "user", Content: "second"},
			},
			want: "[model:openai/gpt-4o] second",
		},
		{
			name:     "no user message returns empty",
			model:    "openai/gpt-4o",
			messages: []types.Message{{Role: "system", Content: "sys"}},
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.ChatCompletionRequest{Model: tc.model, Messages: tc.messages}
			got := buildCacheText(req)
			if got != tc.want {
				t.Errorf("buildCacheText=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestCache_Lookup_EmbedError_NonFatal(t *testing.T) {
	emb := &mockEmbedder{err: errTest("embed failed")}
	c := &Cache{
		embedder:  emb,
		threshold: 0.95,
		logger:    noopLogger(),
	}

	req := &types.ChatCompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	}

	resp, similarity, err := c.Lookup(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error on embed failure, got: %v", err)
	}
	if resp != nil || similarity != 0 {
		t.Error("expected nil response on embed failure")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
