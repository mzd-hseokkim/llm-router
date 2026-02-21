package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/llm-router/gateway/internal/alerting"
	"github.com/llm-router/gateway/internal/audit"
	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/auth/oauth"
	"github.com/llm-router/gateway/internal/auth/rbac"
	"github.com/llm-router/gateway/internal/auth/session"
	"github.com/llm-router/gateway/internal/budget"
	exactcache "github.com/llm-router/gateway/internal/cache/exact"
	semanticcache "github.com/llm-router/gateway/internal/cache/semantic"
	"github.com/llm-router/gateway/internal/config"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/crypto"
	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/handler"
	"github.com/llm-router/gateway/internal/gateway/middleware"
	"github.com/llm-router/gateway/internal/gateway/router"
	"github.com/llm-router/gateway/internal/guardrail"
	"github.com/llm-router/gateway/internal/guardrail/content"
	"github.com/llm-router/gateway/internal/guardrail/injection"
	"github.com/llm-router/gateway/internal/guardrail/keyword"
	"github.com/llm-router/gateway/internal/guardrail/pii"
	"github.com/llm-router/gateway/internal/health"
	internalhandler "github.com/llm-router/gateway/internal/handler"
	"github.com/llm-router/gateway/internal/provider"
	provideranthropic "github.com/llm-router/gateway/internal/provider/anthropic"
	providerazure "github.com/llm-router/gateway/internal/provider/azure"
	providerbedrock "github.com/llm-router/gateway/internal/provider/bedrock"
	providercohere "github.com/llm-router/gateway/internal/provider/cohere"
	providergemini "github.com/llm-router/gateway/internal/provider/gemini"
	providermistral "github.com/llm-router/gateway/internal/provider/mistral"
	provideropenai "github.com/llm-router/gateway/internal/provider/openai"
	"github.com/llm-router/gateway/internal/ratelimit"
	"github.com/llm-router/gateway/internal/server"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
	redistore "github.com/llm-router/gateway/internal/store/redis"
	"github.com/llm-router/gateway/internal/telemetry"
)

const version = "1.0.0"

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Allow standard env vars to override config file keys.
	applyEnvKeys(cfg)

	logger := setupLogger(cfg.Log)

	// --- OpenTelemetry ---
	otelShutdown := telemetry.InitOTel(context.Background(), "llm-router-gateway", version, logger)
	defer func() { _ = otelShutdown(context.Background()) }()

	// --- Database ---
	pool, err := pgstore.NewPool(context.Background(), cfg.Database.URL, int32(cfg.Database.MaxConnections))
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connected")

	// --- Redis ---
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	logger.Info("redis connected")

	// --- Auth ---
	keyStore := pgstore.NewVirtualKeyStore(pool)
	keyCache := auth.NewRedisCache(redisClient)
	authMw := auth.NewVirtualKeyMiddleware(keyStore, keyCache, logger)

	// --- Provider Key Manager ---
	km, cipher := buildKeyManager(pool, cfg, logger)

	// --- Providers ---
	registry := buildRegistry(km, cfg)
	logger.Info("providers registered", "count", len(registry.AllProviders()))

	// --- Circuit Breaker ---
	cbCfg := circuitbreaker.Config{
		FailureThreshold: cfg.Routing.CircuitBreaker.FailureThreshold,
		SuccessThreshold: cfg.Routing.CircuitBreaker.SuccessThreshold,
		OpenTimeout:      cfg.Routing.CircuitBreaker.OpenTimeout,
	}
	cb := circuitbreaker.New(cbCfg)

	// --- Fallback Router ---
	fr := fallback.NewRouter(registry, cb, logger)

	// --- Request logging ---
	logStore := pgstore.NewLogStore(pool)
	logWriter := telemetry.NewLogWriter(logStore, logger)
	defer logWriter.Close()

	// --- Cost calculator ---
	costCalc, err := cost.LoadFromYAML("config/models.yaml")
	if err != nil {
		logger.Warn("failed to load models pricing; cost tracking disabled", "error", err)
		costCalc = cost.NewCalculator(nil)
	} else {
		logger.Info("model pricing loaded")
	}

	// --- Rate limiter ---
	rateLimiter := ratelimit.NewRedisLimiter(redisClient)

	// --- Budget manager ---
	budgetStore := pgstore.NewBudgetStore(pool)
	budgetCache := redistore.NewBudgetCache(redisClient)
	budgetMgr := budget.NewManager(budgetStore, budgetCache, logger)

	budgetScheduler := budget.NewScheduler(budgetMgr, time.Minute, logger)
	budgetScheduler.Start()
	defer budgetScheduler.Stop()

	// --- Audit logger ---
	auditStore := pgstore.NewAuditStore(pool)
	auditLogger := audit.New(auditStore, logger)
	defer auditLogger.Close()

	// --- Provider health tracker ---
	tracker := health.NewProviderTracker()

	// --- Health checker ---
	checker := health.NewChecker(pool, redisClient, version)
	healthHandler := handler.NewHealthHandler(checker, tracker, registry)

	// --- Routing store (hot-reload) ---
	routingStore := handler.NewRoutingStore(cfg.Routing)

	// --- HTTP server ---
	srv := server.New(cfg.Server, logger)
	r := srv.Router()

	// --- Guardrails ---
	guardrailPipeline := buildGuardrailPipeline(cfg, logger)

	// --- Exact-match cache ---
	var cacheMw *middleware.CacheMiddleware
	var ec *exactcache.Cache
	if cfg.Cache.ExactMatch.Enabled {
		ec = exactcache.New(redisClient, cfg.Cache.ExactMatch.DefaultTTL, cfg.Cache.ExactMatch.MaxResponseSize)
		cacheMw = middleware.NewCacheMiddleware(ec, cfg.Cache.ExactMatch.CacheTemperatureZeroOnly)

		// --- Semantic cache (depends on exact cache + pgvector) ---
		if cfg.Cache.Semantic.Enabled {
			apiKey := cfg.Cache.Semantic.EmbeddingAPIKey
			if apiKey == "" {
				apiKey = cfg.Providers.OpenAI.APIKey
			}
			embedder := semanticcache.NewOpenAIEmbedder(apiKey, cfg.Cache.Semantic.EmbeddingModel, cfg.Providers.OpenAI.BaseURL)
			vectorStore := pgstore.NewVectorStore(pool)
			semCache := semanticcache.New(vectorStore, embedder, cfg.Cache.Semantic.Threshold, cfg.Cache.Semantic.TTL, logger)
			cacheMw.WithSemantic(func(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, float64, error) {
				return semCache.Lookup(ctx, req)
			})
			logger.Info("semantic cache enabled", "threshold", cfg.Cache.Semantic.Threshold)
		}
		logger.Info("exact-match cache enabled", "ttl", cfg.Cache.ExactMatch.DefaultTTL)
	}

	// --- Alerting ---
	alertRouter := buildAlertRouter(cfg, redisClient, pool, logger)

	// --- Advanced routing engine ---
	ruleStore := pgstore.NewRoutingRuleStore(pool)
	advRouter := buildAdvancedRouter(ruleStore, costCalc, logger)

	// /v1 routes — protected by virtual key auth
	chatHandler := router.Setup(r, registry, fr, buildFallbackChains(cfg.Routing), logger,
		authMw.Middleware, logWriter, tracker, rateLimiter, budgetMgr, costCalc, cacheMw, guardrailPipeline, advRouter)

	// Subscribe chat handler to routing config changes so that PUT /admin/routing
	// immediately updates the active fallback chains.
	routingStore.Subscribe(func(newCfg config.RoutingConfig) {
		chatHandler.SetChains(buildFallbackChains(newCfg))
		logger.Info("fallback chains reloaded from routing store")
	})

	// /ping — unauthenticated liveness stub (kept for backwards compatibility)
	r.Get("/ping", internalhandler.Ping())

	// /health/* — unauthenticated health endpoints
	r.Get("/health", healthHandler.Full)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)
	r.Get("/health/providers", healthHandler.Providers)

	// /metrics — Prometheus metrics
	r.Handle("/metrics", promhttp.Handler())

	// /admin/* — protected by master key
	orgStore := pgstore.NewOrgStore(pool)
	roleStore := pgstore.NewRoleStore(pool)
	registerAdminRoutes(r, keyStore, keyCache, km, cipher, pool, logStore, cb, cfg, logger,
		rateLimiter, budgetMgr, budgetStore, routingStore, orgStore, roleStore, ec, ruleStore, advRouter,
		auditLogger, auditStore, alertRouter)

	// /auth/* — OAuth / SSO endpoints
	oauthProviders := buildOAuthProviders(cfg)
	if len(oauthProviders) > 0 {
		sessionStore := session.NewStore(redisClient, cfg.Auth.SessionTTL)
		authorizer := rbac.NewAuthorizer(roleStore, redisClient)
		authHandler := handler.NewAuthHandler(oauthProviders, sessionStore, orgStore, roleStore, authorizer, logger)
		r.Get("/auth/providers", authHandler.Providers)
		r.Get("/auth/login", authHandler.Login)
		r.Get("/auth/callback", authHandler.Callback)
		r.Post("/auth/logout", authHandler.Logout)
		r.Group(func(sub chi.Router) {
			sub.Use(authHandler.SessionMiddleware)
			sub.Get("/auth/me", authHandler.Me)
		})
		logger.Info("oauth/sso enabled", "providers", len(oauthProviders))
	}

	if err := srv.ListenAndServe(context.Background()); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

// buildRegistry registers all configured providers.
func buildRegistry(km *provider.KeyManager, cfg *config.Config) *provider.Registry {
	registry := provider.NewRegistry()

	registry.Register(provideropenai.NewManaged(km, cfg.Providers.OpenAI.BaseURL))
	registry.Register(provideranthropic.NewManaged(km, cfg.Providers.Anthropic.BaseURL))
	registry.Register(providergemini.NewManaged(km, cfg.Providers.Gemini.BaseURL))

	if cfg.Providers.Mistral.APIKey != "" || cfg.Providers.Mistral.BaseURL != "" {
		registry.Register(providermistral.NewManaged(km, cfg.Providers.Mistral.BaseURL))
	}

	if cfg.Providers.Cohere.APIKey != "" || cfg.Providers.Cohere.BaseURL != "" {
		registry.Register(providercohere.NewManaged(km, cfg.Providers.Cohere.BaseURL))
	}

	if cfg.Providers.Azure.ResourceName != "" || cfg.Providers.Azure.BaseURL != "" {
		deps := make([]providerazure.DeploymentConfig, 0, len(cfg.Providers.Azure.Deployments))
		for _, d := range cfg.Providers.Azure.Deployments {
			deps = append(deps, providerazure.DeploymentConfig{ID: d.ID, Model: d.Model})
		}
		azureCfg := providerazure.Config{
			ResourceName: cfg.Providers.Azure.ResourceName,
			APIKey:       cfg.Providers.Azure.APIKey,
			APIVersion:   cfg.Providers.Azure.APIVersion,
			BaseURL:      cfg.Providers.Azure.BaseURL,
			Deployments:  deps,
		}
		registry.Register(providerazure.NewManaged(km, azureCfg))
	}

	if cfg.Providers.Bedrock.AccessKeyID != "" {
		bedrockCfg := providerbedrock.Config{
			Region: cfg.Providers.Bedrock.Region,
			Auth: providerbedrock.AuthConfig{
				AccessKeyID:     cfg.Providers.Bedrock.AccessKeyID,
				SecretAccessKey: cfg.Providers.Bedrock.SecretAccessKey,
				SessionToken:    cfg.Providers.Bedrock.SessionToken,
			},
		}
		registry.Register(providerbedrock.New(bedrockCfg))
	}

	return registry
}

// buildFallbackChains converts routing config to fallback.Chain objects.
func buildFallbackChains(rc config.RoutingConfig) []fallback.Chain {
	chains := make([]fallback.Chain, 0, len(rc.FallbackChains))
	for _, fc := range rc.FallbackChains {
		targets := make([]fallback.Target, 0, len(fc.Targets))
		for _, t := range fc.Targets {
			targets = append(targets, fallback.Target{
				Provider: t.Provider,
				Model:    t.Model,
				Weight:   t.Weight,
			})
		}
		chains = append(chains, fallback.Chain{
			Name:    fc.Name,
			Targets: targets,
		})
	}
	return chains
}

// buildKeyManager creates a KeyManager.
// Returns (km, cipher); cipher is nil if ENCRYPTION_KEY is not configured.
func buildKeyManager(pool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger) (*provider.KeyManager, *crypto.Cipher) {
	fallbackKeys := map[string]string{
		"openai":    cfg.Providers.OpenAI.APIKey,
		"anthropic": cfg.Providers.Anthropic.APIKey,
		"gemini":    cfg.Providers.Gemini.APIKey,
		"mistral":   cfg.Providers.Mistral.APIKey,
		"cohere":    cfg.Providers.Cohere.APIKey,
		"azure":     cfg.Providers.Azure.APIKey,
	}

	if cfg.Gateway.EncryptionKey == "" {
		logger.Warn("ENCRYPTION_KEY not set; DB provider key storage disabled, using config file keys only")
		return provider.NewKeyManager(nil, nil, fallbackKeys), nil
	}

	keyBytes, err := base64.StdEncoding.DecodeString(cfg.Gateway.EncryptionKey)
	if err != nil {
		logger.Error("invalid ENCRYPTION_KEY (must be base64-encoded 32 bytes)", "error", err)
		os.Exit(1)
	}

	cipher, err := crypto.NewCipher(keyBytes)
	if err != nil {
		logger.Error("failed to create cipher from ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	providerKeyStore := pgstore.NewProviderKeyStore(pool)
	km := provider.NewKeyManager(providerKeyStore, cipher, fallbackKeys)
	logger.Info("provider key manager initialised with DB store")
	return km, cipher
}

// registerAdminRoutes mounts all /admin/* endpoints.
func registerAdminRoutes(
	r chi.Router,
	store auth.Store,
	cache auth.Cache,
	km *provider.KeyManager,
	cipher *crypto.Cipher,
	pool *pgxpool.Pool,
	logStore *pgstore.LogStore,
	cb *circuitbreaker.CircuitBreaker,
	cfg *config.Config,
	logger *slog.Logger,
	rateLimiter *ratelimit.RedisLimiter,
	budgetMgr *budget.Manager,
	budgetStore *pgstore.BudgetStore,
	routingStore *handler.RoutingStore,
	orgStore *pgstore.OrgStore,
	roleStore *pgstore.RoleStore,
	exactCache *exactcache.Cache,
	ruleStore *pgstore.RoutingRuleStore,
	advRouter *router.AdvancedRouter,
	auditLogger *audit.Logger,
	auditStore *pgstore.AuditStore,
	alertRouter *alerting.Router,
) {
	adminMw := auth.AdminAuth(cfg.Gateway.MasterKey)

	r.Group(func(r chi.Router) {
		r.Use(adminMw)
		if auditLogger != nil {
			r.Use(audit.Middleware(auditLogger))
		}

		// Virtual key CRUD
		vkHandler := handler.NewAdminKeysHandler(store, cache, logger)
		r.Post("/admin/keys", vkHandler.Create)
		r.Get("/admin/keys", vkHandler.List)
		r.Get("/admin/keys/{id}", vkHandler.Get)
		r.Patch("/admin/keys/{id}", vkHandler.Update)
		r.Delete("/admin/keys/{id}", vkHandler.Deactivate)
		r.Post("/admin/keys/{id}/regenerate", vkHandler.Regenerate)

		// Provider key CRUD (only available when encryption is configured)
		if cipher != nil {
			pkStore := pgstore.NewProviderKeyStore(pool)
			pkHandler := handler.NewAdminProviderKeysHandler(pkStore, km, cipher, logger)
			r.Post("/admin/provider-keys", pkHandler.Create)
			r.Get("/admin/provider-keys", pkHandler.List)
			r.Get("/admin/provider-keys/{id}", pkHandler.Get)
			r.Put("/admin/provider-keys/{id}", pkHandler.Update)
			r.Delete("/admin/provider-keys/{id}", pkHandler.Delete)
			r.Put("/admin/provider-keys/{id}/rotate", pkHandler.Rotate)
		}

		// Request log query
		logsHandler := handler.NewAdminLogsHandler(logStore)
		r.Get("/admin/logs", logsHandler.List)
		r.Get("/admin/logs/{request_id}", logsHandler.Get)

		// Circuit breaker admin endpoints
		cbHandler := handler.NewAdminCircuitBreakerHandler(cb, logger)
		r.Get("/admin/circuit-breakers", cbHandler.List)
		r.Post("/admin/circuit-breakers/{provider}/reset", cbHandler.Reset)

		// Rate limit admin endpoints
		rlHandler := handler.NewAdminRateLimitsHandler(store, rateLimiter)
		r.Get("/admin/rate-limits/{id}", rlHandler.Get)
		r.Post("/admin/rate-limits/{id}/reset", rlHandler.Reset)

		// Budget admin endpoints
		budgetHandler := handler.NewAdminBudgetsHandler(budgetMgr, budgetStore, logger)
		r.Post("/admin/budgets", budgetHandler.Create)
		r.Get("/admin/budgets/{entity_type}/{entity_id}", budgetHandler.List)
		r.Post("/admin/budgets/{id}/reset", budgetHandler.Reset)

		// Usage summary endpoints
		usageStore := pgstore.NewUsageStore(pool)
		usageHandler := handler.NewAdminUsageHandler(usageStore)
		r.Get("/admin/usage/summary", usageHandler.Summary)
		r.Get("/admin/usage/top-spenders", usageHandler.TopSpenders)

		// Organizations, Teams, Users
		orgsHandler := handler.NewAdminOrgsHandler(orgStore)
		r.Post("/admin/organizations", orgsHandler.CreateOrg)
		r.Get("/admin/organizations", orgsHandler.ListOrgs)
		r.Get("/admin/organizations/{id}", orgsHandler.GetOrg)
		r.Put("/admin/organizations/{id}", orgsHandler.UpdateOrg)
		r.Post("/admin/teams", orgsHandler.CreateTeam)
		r.Get("/admin/teams", orgsHandler.ListTeams)
		r.Get("/admin/teams/{id}", orgsHandler.GetTeam)
		r.Put("/admin/teams/{id}", orgsHandler.UpdateTeam)
		r.Post("/admin/users", orgsHandler.CreateUser)
		r.Get("/admin/users", orgsHandler.ListUsers)
		r.Get("/admin/users/{id}", orgsHandler.GetUser)
		r.Put("/admin/users/{id}", orgsHandler.UpdateUser)

		// Role management (RBAC)
		rolesHandler := handler.NewAdminRolesHandler(roleStore, rbac.NewAuthorizer(roleStore, nil))
		r.Get("/admin/roles", rolesHandler.ListRoles)
		r.Post("/admin/users/{user_id}/roles", rolesHandler.AssignRole)

		// Routing config (hot reload) — routingStore is shared with main() subscribers
		routingHandler := handler.NewAdminRoutingHandler(routingStore)
		r.Get("/admin/routing", routingHandler.Get)
		r.Put("/admin/routing", routingHandler.Update)
		r.Post("/admin/routing/reload", routingHandler.Reload)

		// Advanced routing rules CRUD
		if ruleStore != nil {
			rrHandler := handler.NewAdminRoutingRulesHandler(ruleStore, advRouter)
			r.Get("/admin/routing/rules", rrHandler.List)
			r.Post("/admin/routing/rules", rrHandler.Create)
			r.Put("/admin/routing/rules/{id}", rrHandler.UpdateRule)
			r.Delete("/admin/routing/rules/{id}", rrHandler.DeleteRule)
			r.Post("/admin/routing/rules/reload", rrHandler.ReloadRules)
			r.Post("/admin/routing/test", rrHandler.DryRun)
		}

		// Cache admin endpoints
		if exactCache != nil {
			cacheHandler := handler.NewAdminCacheHandler(exactCache)
			r.Delete("/admin/cache/exact", cacheHandler.Delete)
			r.Get("/admin/cache/exact/{hash}", cacheHandler.Get)
		}

		// Audit log endpoints
		if auditStore != nil {
			auditHandler := handler.NewAdminAuditHandler(auditStore)
			r.Get("/admin/audit-logs", auditHandler.List)
			r.Get("/admin/audit-logs/security-events", auditHandler.SecurityEvents)
		}

		// Alerting endpoints
		if alertRouter != nil {
			alertsHandler := handler.NewAdminAlertsHandler(alertRouter, pool)
			r.Post("/admin/alerts/test", alertsHandler.Test)
			r.Get("/admin/alerts/history", alertsHandler.History)
		}

		// OpenAPI spec
		openAPIHandler := handler.NewAdminOpenAPIHandler()
		r.Get("/admin/openapi.json", openAPIHandler.Spec)
	})
}

// buildOAuthProviders creates OAuth provider instances from config.
func buildOAuthProviders(cfg *config.Config) []oauth.Provider {
	var providers []oauth.Provider
	for _, pc := range cfg.Auth.Providers {
		if !pc.IsEnabled() {
			continue
		}
		provCfg := oauth.Config{
			ClientID:         pc.ClientID,
			ClientSecret:     pc.ClientSecret,
			GroupRoleMapping: pc.GroupRoleMapping,
		}
		switch pc.Name {
		case "google":
			providers = append(providers, oauth.NewGoogle(provCfg))
		case "github":
			providers = append(providers, oauth.NewGitHub(provCfg))
		default:
			// OIDC providers require network discovery — skip if issuer URL missing
			if pc.IssuerURL == "" {
				continue
			}
			p, err := oauth.NewOIDC(context.Background(), pc.Name, provCfg, pc.IssuerURL)
			if err == nil {
				providers = append(providers, p)
			}
		}
	}
	return providers
}

// applyEnvKeys overrides provider API keys and gateway config from env vars.
func applyEnvKeys(cfg *config.Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.Providers.OpenAI.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.Providers.Anthropic.APIKey = v
	}
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.Providers.Gemini.APIKey = v
	}
	if v := os.Getenv("MISTRAL_API_KEY"); v != "" {
		cfg.Providers.Mistral.APIKey = v
	}
	if v := os.Getenv("COHERE_API_KEY"); v != "" {
		cfg.Providers.Cohere.APIKey = v
	}
	if v := os.Getenv("AZURE_API_KEY"); v != "" {
		cfg.Providers.Azure.APIKey = v
	}
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		cfg.Providers.Bedrock.AccessKeyID = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		cfg.Providers.Bedrock.SecretAccessKey = v
	}
	if v := os.Getenv("AWS_SESSION_TOKEN"); v != "" {
		cfg.Providers.Bedrock.SessionToken = v
	}
	if v := os.Getenv("MASTER_KEY"); v != "" {
		cfg.Gateway.MasterKey = v
	}
	if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
		cfg.Gateway.EncryptionKey = v
	}
}

// buildGuardrailPipeline constructs the guardrail pipeline from config.
// Returns nil if no guardrails are enabled.
func buildGuardrailPipeline(cfg *config.Config, logger *slog.Logger) *guardrail.Pipeline {
	gc := cfg.Guardrails

	var inputGuards, outputGuards []guardrail.Guardrail

	// PII: applied to both input and output
	if gc.PII.Enabled {
		action := guardrail.Action(gc.PII.Action)
		d := pii.NewDetector(true, action, gc.PII.Categories)
		inputGuards = append(inputGuards, d)
		outputGuards = append(outputGuards, d)
	}

	// Prompt injection: input only
	if gc.PromptInjection.Enabled {
		d := injection.NewDetector(true, guardrail.Action(gc.PromptInjection.Action))
		inputGuards = append(inputGuards, d)
	}

	// Content filter: both directions
	if gc.ContentFilter.Enabled {
		f := content.NewFilter(true, guardrail.Action(gc.ContentFilter.Action), gc.ContentFilter.Categories)
		inputGuards = append(inputGuards, f)
		outputGuards = append(outputGuards, f)
	}

	// Custom keywords: both directions
	if gc.CustomKeywords.Enabled && len(gc.CustomKeywords.Blocked) > 0 {
		f := keyword.NewFilter(true, guardrail.Action(gc.CustomKeywords.Action), gc.CustomKeywords.Blocked)
		inputGuards = append(inputGuards, f)
		outputGuards = append(outputGuards, f)
	}

	if len(inputGuards) == 0 && len(outputGuards) == 0 {
		return nil
	}

	logger.Info("guardrails enabled",
		"input_count", len(inputGuards),
		"output_count", len(outputGuards))
	return guardrail.NewPipeline(inputGuards, outputGuards, logger)
}

// buildAlertRouter creates the alerting Router from config.
// Returns nil if no channels are configured (graceful degradation).
func buildAlertRouter(cfg *config.Config, redisClient *redis.Client, pool *pgxpool.Pool, logger *slog.Logger) *alerting.Router {
	if len(cfg.Alerting.Channels) == 0 {
		return nil
	}

	notifiers := make(map[string]alerting.Notifier, len(cfg.Alerting.Channels))
	for _, ch := range cfg.Alerting.Channels {
		switch ch.Type {
		case "slack":
			notifiers[ch.Name] = alerting.NewSlackNotifier(ch.Name, ch.WebhookURL)
		case "webhook":
			url := ch.URL
			if url == "" {
				url = ch.WebhookURL
			}
			notifiers[ch.Name] = alerting.NewWebhookNotifier(ch.Name, url, ch.Method, ch.Headers, ch.Retry)
		case "email":
			notifiers[ch.Name] = alerting.NewEmailNotifier(ch.Name, ch.SMTPHost, ch.SMTPPort, ch.From, ch.To)
		default:
			logger.Warn("alerting: unknown channel type", "name", ch.Name, "type", ch.Type)
		}
	}

	dedup := alerting.NewDeduplicator(redisClient, 15*time.Minute)
	ar := alerting.NewRouter(cfg.Alerting, notifiers, dedup, pool, logger)
	logger.Info("alerting enabled", "channels", len(notifiers))
	return ar
}

// buildAdvancedRouter loads routing rules from DB and creates the engine.
// Returns nil if the table is not available or the load fails (graceful degradation).
func buildAdvancedRouter(store *pgstore.RoutingRuleStore, pricing *cost.Calculator, logger *slog.Logger) *router.AdvancedRouter {
	rules, err := store.List(context.Background())
	if err != nil {
		logger.Warn("failed to load routing rules; advanced routing disabled", "error", err)
		return nil
	}
	ar, err := router.NewAdvancedRouter(rules, pricing)
	if err != nil {
		logger.Warn("failed to build advanced router", "error", err)
		return nil
	}
	logger.Info("advanced routing engine loaded", "rules", len(rules))
	return ar
}

func setupLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var h slog.Handler
	if cfg.Format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(h)
}
