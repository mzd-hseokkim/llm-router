package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/gateway/handler"
	"github.com/llm-router/gateway/internal/gateway/middleware"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

// Setup registers all /v1 API routes on the given chi router.
// authMw is the virtual-key middleware injected from main; it replaces the stub.
// logWriter, when non-nil, enables per-request DB logging.
// recorder, when non-nil, receives provider health events for tracking.
func Setup(r chi.Router, registry *provider.Registry, logger *slog.Logger, authMw func(http.Handler) http.Handler, logWriter *telemetry.LogWriter, recorder middleware.RequestRecorder) {
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.RequestMeta)

	if logWriter != nil {
		r.Use(middleware.RequestLogger(logWriter, logger, recorder))
	}

	r.Route("/v1", func(r chi.Router) {
		r.Use(authMw)

		chat := handler.NewChatHandler(registry, logger)
		r.Post("/chat/completions", chat.Handle)

		comp := handler.NewCompletionsHandler(registry, logger)
		r.Post("/completions", comp.Handle)

		emb := handler.NewEmbeddingsHandler(registry, logger)
		r.Post("/embeddings", emb.Handle)

		models := handler.NewModelsHandler(registry, logger)
		r.Get("/models", models.List)
		r.Get("/models/*", models.Get) // wildcard handles model IDs with slashes
	})
}
