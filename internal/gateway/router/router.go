package router

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/llm-router/gateway/internal/budget"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/handler"
	"github.com/llm-router/gateway/internal/gateway/middleware"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/guardrail"
	"github.com/llm-router/gateway/internal/prompt"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/ratelimit"
	"github.com/llm-router/gateway/internal/residency"
	"github.com/llm-router/gateway/internal/telemetry"
)

// AdvancedResolverIface is implemented by any component that can resolve a
// fallback.Chain from a chat completion request. This allows both the
// rule-based AdvancedRouter and the ML-based Router to be used interchangeably.
type AdvancedResolverIface interface {
	Resolve(ctx context.Context, req *types.ChatCompletionRequest) (fallback.Chain, bool)
}

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
	advancedRouter AdvancedResolverIface,
	promptSvc *prompt.Service,
	abTestMw *middleware.ABTestMiddleware,
	residencyEnforcer *residency.Enforcer,
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

		// Data residency: extract policy from request header and store in context.
		if residencyEnforcer != nil {
			r.Use(middleware.DataResidency())
		}

		// Prompt injection middleware
		if promptSvc != nil {
			r.Use(middleware.PromptInjector(promptSvc, logger))
		}

		// A/B test middleware — must run after auth so entity ID is resolvable
		if abTestMw != nil {
			r.Use(abTestMw.Handler())
		}

		chat = handler.NewChatHandler(fr, logger).WithChains(chains)
		if advancedRouter != nil {
			chat = chat.WithAdvancedRouter(advancedRouter)
		}
		if residencyEnforcer != nil {
			chat = chat.WithResidencyEnforcer(residencyEnforcer)
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
