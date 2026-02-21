package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/budget"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/handler"
	"github.com/llm-router/gateway/internal/gateway/middleware"
	"github.com/llm-router/gateway/internal/guardrail"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/ratelimit"
	"github.com/llm-router/gateway/internal/telemetry"
)

// Setup registers all /v1 API routes on the given chi router.
// Returns the ChatHandler so callers can subscribe to routing config changes
// via ChatHandler.SetChains.
func Setup(
	r chi.Router,
	registry *provider.Registry,
	fr *fallback.Router,
	chains []fallback.Chain,
	logger *slog.Logger,
	authMw func(http.Handler) http.Handler,
	logWriter *telemetry.LogWriter,
	recorder middleware.RequestRecorder,
	rateLimiter ratelimit.Limiter,
	budgetMgr *budget.Manager,
	costCalc *cost.Calculator,
	cacheMw *middleware.CacheMiddleware,
	guardrailPipeline *guardrail.Pipeline,
	advancedRouter *AdvancedRouter,
) *handler.ChatHandler {
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.RequestMeta)

	if logWriter != nil {
		r.Use(middleware.RequestLogger(logWriter, logger, recorder))
	}

	var chat *handler.ChatHandler
	r.Route("/v1", func(r chi.Router) {
		r.Use(authMw)

		if rateLimiter != nil {
			r.Use(middleware.RateLimit(rateLimiter))
		}

		if budgetMgr != nil && costCalc != nil {
			r.Use(middleware.BudgetCheck(budgetMgr, costCalc, logger))
		}

		// Cache middleware (before guardrails so cached responses skip guardrail processing)
		if cacheMw != nil {
			r.Use(cacheMw.Handler())
		}

		// Guardrail middleware
		if guardrailPipeline != nil {
			r.Use(middleware.GuardrailCheck(guardrailPipeline))
		}

		chat = handler.NewChatHandler(fr, logger).WithChains(chains)
		if advancedRouter != nil {
			chat = chat.WithAdvancedRouter(advancedRouter)
		}
		r.Post("/chat/completions", chat.Handle)

		comp := handler.NewCompletionsHandler(registry, logger)
		r.Post("/completions", comp.Handle)

		emb := handler.NewEmbeddingsHandler(registry, logger)
		r.Post("/embeddings", emb.Handle)

		models := handler.NewModelsHandler(registry, logger)
		r.Get("/models", models.List)
		r.Get("/models/*", models.Get) // wildcard handles model IDs with slashes
	})

	return chat
}
