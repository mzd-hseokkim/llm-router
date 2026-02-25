package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/llm-router/gateway/internal/alerting"
	"github.com/llm-router/gateway/internal/audit"
	"github.com/llm-router/gateway/internal/mcp"
	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/residency"
	"github.com/llm-router/gateway/internal/auth/oauth"
	"github.com/llm-router/gateway/internal/auth/rbac"
	"github.com/llm-router/gateway/internal/auth/session"
	"github.com/llm-router/gateway/internal/budget"
	exactcache "github.com/llm-router/gateway/internal/cache/exact"
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
	"github.com/llm-router/gateway/internal/guardrail/llmjudge"
	"github.com/llm-router/gateway/internal/guardrail/pii"
	"github.com/llm-router/gateway/internal/billing"
	"github.com/llm-router/gateway/internal/prompt"
	"github.com/llm-router/gateway/internal/health"
	"github.com/llm-router/gateway/internal/mlrouter"
	internalhandler "github.com/llm-router/gateway/internal/handler"
	"github.com/llm-router/gateway/internal/provider"
	provideranthropic "github.com/llm-router/gateway/internal/provider/anthropic"
	providerazure "github.com/llm-router/gateway/internal/provider/azure"
	providerbedrock "github.com/llm-router/gateway/internal/provider/bedrock"
	providercohere "github.com/llm-router/gateway/internal/provider/cohere"
	providergemini "github.com/llm-router/gateway/internal/provider/gemini"
	providergrok "github.com/llm-router/gateway/internal/provider/grok"
	providermistral "github.com/llm-router/gateway/internal/provider/mistral"
	provideropenai "github.com/llm-router/gateway/internal/provider/openai"
	providerselfhosted "github.com/llm-router/gateway/internal/provider/selfhosted"
	"golang.org/x/crypto/bcrypt"

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
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
	})
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

	// --- Admin JWT Auth ---
	adminCredStore := pgstore.NewAdminCredentialStore(pool)
	// Seed default admin account with master key as initial password (bcrypt).
	defaultHash, err := bcrypt.GenerateFromPassword([]byte(cfg.Gateway.MasterKey), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("admin seeding: bcrypt failed", "error", err)
		os.Exit(1)
	}
	if err := adminCredStore.UpsertDefault(context.Background(), "admin", string(defaultHash)); err != nil {
		logger.Error("admin seeding failed", "error", err)
		os.Exit(1)
	}
	logger.Info("default admin account ready")
	keySum := sha256.Sum256([]byte(cfg.Gateway.MasterKey))
	jwtSvc := auth.NewJWTService(keySum[:], 24*time.Hour)

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
	guardrailPolicyStore := pgstore.NewGuardrailPolicyStore(pool)
	guardrailMgr := guardrail.NewManager(logger)
	initGuardrails(context.Background(), guardrailPolicyStore, guardrailMgr, cfg, registry, logger)

	// --- Exact-match cache (temperature=0 only) ---
	var cacheMw *middleware.CacheMiddleware
	var ec *exactcache.Cache
	if cfg.Cache.ExactMatch.Enabled {
		ec = exactcache.New(redisClient, cfg.Cache.ExactMatch.DefaultTTL, cfg.Cache.ExactMatch.MaxResponseSize)
		cacheMw = middleware.NewCacheMiddleware(ec)
		logger.Info("exact-match cache enabled", "ttl", cfg.Cache.ExactMatch.DefaultTTL)
	}

	// --- Alerting ---
	alertRouter := buildAlertRouter(cfg, redisClient, pool, logger)

	// --- Advanced routing engine ---
	ruleStore := pgstore.NewRoutingRuleStore(pool)
	advRouter := buildAdvancedRouter(ruleStore, costCalc, logger)

	// --- ML routing engine ---
	mlRouter := buildMLRouter(cfg, costCalc, tracker, logger)
	if mlRouter != nil {
		defer mlRouter.Close()
	}

	// --- Prompt management ---
	promptStore := pgstore.NewPromptStore(pool)
	promptSvc := prompt.NewService(promptStore)

	// --- A/B testing ---
	abTestStore := pgstore.NewABTestStore(pool)
	abTestMw := middleware.NewABTestMiddleware(abTestStore, logger)
	if err := abTestMw.Reload(context.Background()); err != nil {
		logger.Warn("failed to load ab tests; a/b testing disabled", "error", err)
	}

	// --- Billing / Chargeback ---
	billingStore := pgstore.NewBillingStore(pool)
	chargebackSvc := billing.NewChargebackService(billingStore)
	billingScheduler := billing.NewScheduler(chargebackSvc, nil, logger)
	billingScheduler.Start()
	defer billingScheduler.Stop()

	// --- Data Residency ---
	residencyEnforcer := buildResidencyEnforcer(cfg)

	// Prefer rule-based advanced router; fall back to ML router if available.
	var activeAdvRouter router.AdvancedResolverIface = advRouter
	if activeAdvRouter == nil && mlRouter != nil {
		activeAdvRouter = mlRouter // live or shadow mode
	}

	// /v1 routes — protected by virtual key auth
	chatHandler := router.Setup(r, registry, fr, buildFallbackChains(cfg.Routing), logger,
		authMw.Middleware, logWriter, tracker, rateLimiter, budgetMgr, costCalc, cacheMw, guardrailMgr, activeAdvRouter, promptSvc, abTestMw, residencyEnforcer, ec)

	// Subscribe chat handler to routing config changes so that PUT /admin/routing
	// immediately updates the active fallback chains.
	routingStore.Subscribe(func(newCfg config.RoutingConfig) {
		chatHandler.SetChains(buildFallbackChains(newCfg))
		logger.Info("fallback chains reloaded from routing store")
	})

	// /mcp/v1/* — MCP Gateway Hub (protected by virtual key auth)
	if cfg.MCP.Enabled {
		mcpHub := buildMCPHub(context.Background(), cfg, auditLogger, logger)
		if mcpHub != nil {
			defer mcpHub.Close()
			registerMCPRoutes(r, mcpHub, cfg, logger, authMw.Middleware, auditLogger, jwtSvc)
		}
	}

	// /docs — Swagger UI (unauthenticated; spec itself is public)
	docsHandler := handler.NewDocsHandler()
	r.Get("/docs", docsHandler.UI)
	r.Get("/docs/openapi.json", docsHandler.JSON)
	r.Get("/docs/openapi.yaml", docsHandler.YAML)

	// /ping — unauthenticated liveness stub (kept for backwards compatibility)
	r.Get("/ping", internalhandler.Ping())

	// /health/* — unauthenticated health endpoints
	r.Get("/health", healthHandler.Full)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)
	r.Get("/health/providers", healthHandler.Providers)

	// /metrics — Prometheus metrics
	r.Handle("/metrics", promhttp.Handler())

	// Sync providers/models from DB into registry and pricing table (best-effort).
	syncProvidersFromDB(context.Background(), pool, registry, costCalc, logger)

	// /admin/* — protected by JWT
	orgStore := pgstore.NewOrgStore(pool)
	roleStore := pgstore.NewRoleStore(pool)
	registerAdminRoutes(r, keyStore, keyCache, km, cipher, pool, logStore, cb, cfg, logger,
		rateLimiter, budgetMgr, budgetStore, routingStore, orgStore, roleStore, ec, ruleStore, advRouter,
		auditLogger, auditStore, alertRouter, promptStore, promptSvc,
		abTestStore, abTestMw, billingStore, chargebackSvc, residencyEnforcer, buildResidencyRegistry(cfg), fr, costCalc, registry,
		guardrailPolicyStore, guardrailMgr, jwtSvc, adminCredStore)

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

	if cfg.Providers.Grok.APIKey != "" || cfg.Providers.Grok.BaseURL != "" {
		registry.Register(providergrok.NewManaged(km, cfg.Providers.Grok.BaseURL))
	}

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

	// Self-hosted inference servers (Ollama, vLLM, TGI, LMStudio)
	for _, sh := range cfg.Providers.SelfHosted {
		if sh.Name == "" || sh.BaseURL == "" {
			continue
		}
		models := make([]providerselfhosted.ModelEntry, 0, len(sh.Models))
		for _, m := range sh.Models {
			models = append(models, providerselfhosted.ModelEntry{
				ID:        m.ID,
				ModelName: m.ModelName,
			})
		}
		adapter := providerselfhosted.New(sh.Name, sh.BaseURL, providerselfhosted.Engine(sh.Engine), models)
		registry.Register(adapter)
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
		"grok":      cfg.Providers.Grok.APIKey,
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
	store *pgstore.VirtualKeyStore,
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
	promptStore *pgstore.PromptStore,
	promptSvc *prompt.Service,
	abTestStore *pgstore.ABTestStore,
	abTestMw *middleware.ABTestMiddleware,
	billingStore *pgstore.BillingStore,
	chargebackSvc *billing.ChargebackService,
	residencyEnforcer *residency.Enforcer,
	residencyRegistry *residency.Registry,
	fr *fallback.Router,
	costCalc *cost.Calculator,
	registry *provider.Registry,
	guardrailPolicyStore guardrail.PolicyStore,
	guardrailMgr *guardrail.Manager,
	jwtSvc *auth.JWTService,
	adminCredStore *pgstore.AdminCredentialStore,
) {
	adminMw := auth.AdminAuth(jwtSvc)

	// Login endpoint — no auth required
	adminAuthHandler := handler.NewAdminAuthHandler(adminCredStore, jwtSvc, rateLimiter, logger)
	r.Post("/admin/auth/login", adminAuthHandler.Login)

	r.Group(func(r chi.Router) {
		r.Use(adminMw)
		if auditLogger != nil {
			r.Use(audit.Middleware(auditLogger))
		}

		// Admin auth endpoints (JWT required)
		r.Post("/admin/auth/change-password", adminAuthHandler.ChangePassword)
		r.Get("/admin/auth/me", adminAuthHandler.Me)

		// Virtual key CRUD
		vkHandler := handler.NewAdminKeysHandler(store, cache, logger)
		r.Post("/admin/keys", vkHandler.Create)
		r.Get("/admin/keys", vkHandler.List)
		r.Get("/admin/keys/{id}", vkHandler.Get)
		r.Patch("/admin/keys/{id}", vkHandler.Update)
		r.Delete("/admin/keys/{id}", vkHandler.Deactivate)
		r.Post("/admin/keys/{id}/regenerate", vkHandler.Regenerate)

		// Provider key CRUD (only available when encryption is configured)
		pkStore := pgstore.NewProviderKeyStore(pool)
		if cipher != nil {
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
		cacheHandler := handler.NewAdminCacheHandler(exactCache, logStore)
		r.Get("/admin/cache/stats", cacheHandler.Stats)
		if exactCache != nil {
			r.Delete("/admin/cache/exact", cacheHandler.Delete)
			r.Get("/admin/cache/exact/{hash}", cacheHandler.Get)
		}

		// Audit log endpoints
		if auditStore != nil {
			auditHandler := handler.NewAdminAuditHandler(auditStore)
			r.Get("/admin/audit-logs", auditHandler.List)
			r.Get("/admin/audit-logs/security-events", auditHandler.SecurityEvents)
		}

		// Alerting endpoints — history and config are always available.
		alertsHandler := handler.NewAdminAlertsHandler(alertRouter, pool)
		r.Get("/admin/alerts/history", alertsHandler.History)
		r.Get("/admin/alerts/config", alertsHandler.GetConfig)
		r.Put("/admin/alerts/config", alertsHandler.UpdateConfig)
		if alertRouter != nil {
			r.Post("/admin/alerts/test", alertsHandler.Test)
		}

		// Prompt management
		if promptStore != nil {
			promptsHandler := handler.NewAdminPromptsHandler(promptSvc, promptStore)
			r.Get("/admin/prompts", promptsHandler.List)
			r.Post("/admin/prompts", promptsHandler.Create)
			r.Get("/admin/prompts/{slug}", promptsHandler.Get)
			r.Get("/admin/prompts/{slug}/versions", promptsHandler.ListVersions)
			r.Post("/admin/prompts/{slug}/versions", promptsHandler.PublishVersion)
			r.Get("/admin/prompts/{slug}/versions/{version}", promptsHandler.GetVersion)
			r.Post("/admin/prompts/{slug}/rollback/{version}", promptsHandler.Rollback)
			r.Post("/admin/prompts/{slug}/render", promptsHandler.Render)
			r.Get("/admin/prompts/{slug}/diff", promptsHandler.Diff)
		}

		// A/B testing
		if abTestStore != nil {
			abHandler := handler.NewAdminABTestsHandler(abTestStore, abTestMw)
			r.Post("/admin/ab-tests", abHandler.Create)
			r.Get("/admin/ab-tests", abHandler.List)
			r.Get("/admin/ab-tests/{id}", abHandler.Get)
			r.Get("/admin/ab-tests/{id}/results", abHandler.Results)
			r.Post("/admin/ab-tests/{id}/start", abHandler.Start)
			r.Post("/admin/ab-tests/{id}/pause", abHandler.Pause)
			r.Post("/admin/ab-tests/{id}/stop", abHandler.Stop)
			r.Post("/admin/ab-tests/{id}/promote", abHandler.Promote)
		}

		// Chargeback / Showback reports
		if billingStore != nil {
			reportsHandler := handler.NewAdminReportsHandler(chargebackSvc, billingStore)
			r.Get("/admin/reports/chargeback", reportsHandler.Chargeback)
			r.Get("/admin/reports/showback", reportsHandler.Showback)
			r.Get("/admin/billing/markup", reportsHandler.GetMarkup)
			r.Put("/admin/billing/markup", reportsHandler.UpsertMarkup)

			billingAPIHandler := handler.NewBillingAPIHandler(billingStore)
			r.Get("/api/billing/usage", billingAPIHandler.Usage)
		}

		// Data residency policy management
		if residencyRegistry != nil {
			drHandler := handler.NewAdminResidencyHandler(residencyRegistry, residencyEnforcer)
			r.Get("/admin/data-residency/policies", drHandler.List)
			r.Get("/admin/data-residency/policies/{name}", drHandler.Get)
			r.Post("/admin/data-residency/validate", drHandler.Validate)
			r.Get("/admin/data-residency/report", drHandler.Report)
		}

		// Provider and model management
		provStore := pgstore.NewProviderStore(pool)
		modelStore := pgstore.NewModelStore(pool)
		providersHandler := handler.NewAdminProvidersHandler(provStore, modelStore, registry, costCalc, cipher, km, pool, logger)
		r.Get("/admin/providers", providersHandler.ListProviders)
		r.Post("/admin/providers", providersHandler.CreateProvider)
		r.Get("/admin/providers/{id}", providersHandler.GetProvider)
		r.Put("/admin/providers/{id}", providersHandler.UpdateProvider)
		r.Delete("/admin/providers/{id}", providersHandler.DeleteProvider)
		r.Get("/admin/providers/{id}/models", providersHandler.ListModels)
		r.Post("/admin/providers/{id}/models", providersHandler.CreateModel)
		r.Put("/admin/providers/{id}/models/{modelId}", providersHandler.UpdateModel)
		r.Delete("/admin/providers/{id}/models/{modelId}", providersHandler.DeleteModel)

		// Guardrail policy management
		guardrailsHandler := handler.NewAdminGuardrailsHandler(
			guardrailPolicyStore, guardrailMgr, logger,
			func(recs []*guardrail.PolicyRecord) *guardrail.Pipeline {
				return buildPipelineFromRecords(recs, cfg, registry, logger)
			},
		)
		r.Get("/admin/guardrails", guardrailsHandler.List)
		r.Get("/admin/guardrails/{type}", guardrailsHandler.Get)
		r.Put("/admin/guardrails/{type}", guardrailsHandler.Update)
		r.Put("/admin/guardrails", guardrailsHandler.UpdateAll)

		// Admin playground (master key auth; no virtual key required)
		playgroundHandler := handler.NewAdminPlaygroundHandler(fr, store, costCalc, logger)
		r.Post("/admin/playground", playgroundHandler.Chat)

		// OpenAPI spec
		openAPIHandler := handler.NewAdminOpenAPIHandler()
		r.Get("/admin/openapi.json", openAPIHandler.Spec)
		r.Get("/admin/openapi.yaml", openAPIHandler.SpecYAML)
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
	if v := os.Getenv("GROK_API_KEY"); v != "" {
		cfg.Providers.Grok.APIKey = v
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

// initGuardrails initialises the GuardrailManager from DB, seeding from config if the
// table is empty.
func initGuardrails(
	ctx context.Context,
	store guardrail.PolicyStore,
	mgr *guardrail.Manager,
	cfg *config.Config,
	registry *provider.Registry,
	logger *slog.Logger,
) {
	recs, err := store.List(ctx)
	if err != nil {
		logger.Warn("guardrails: failed to load policies from DB; using config", "error", err)
		mgr.SetPipeline(buildPipelineFromConfig(cfg, registry, logger))
		return
	}

	if len(recs) == 0 {
		// Seed the DB from config values so the admin UI has records to display.
		seedRecs := guardrail.ConfigToRecords(cfg.Guardrails)
		if err := store.UpsertAll(ctx, seedRecs); err != nil {
			logger.Warn("guardrails: failed to seed DB; using config pipeline", "error", err)
			mgr.SetPipeline(buildPipelineFromConfig(cfg, registry, logger))
			return
		}
		logger.Info("guardrails: seeded policies from config", "count", len(seedRecs))
		recs = seedRecs
	}

	mgr.SetPipeline(buildPipelineFromRecords(recs, cfg, registry, logger))
}

// providerCompleter adapts provider.Provider to the llmjudge.ChatCompleter interface.
type providerCompleter struct {
	p provider.Provider
}

func (c *providerCompleter) Complete(ctx context.Context, system, userMsg, model string) (string, error) {
	maxTokens := 64
	req := &types.ChatCompletionRequest{
		Model: model,
		Messages: []types.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: userMsg},
		},
		MaxTokens: &maxTokens,
	}
	resp, err := c.p.ChatCompletion(ctx, model, req, nil)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("llm judge: empty response from provider")
	}
	return resp.Choices[0].Message.Content, nil
}

// buildPipelineFromRecords constructs a Pipeline from PolicyRecord objects.
// Returns nil if no records are enabled.
func buildPipelineFromRecords(recs []*guardrail.PolicyRecord, cfg *config.Config, registry *provider.Registry, logger *slog.Logger) *guardrail.Pipeline {
	var inputGuards, outputGuards []guardrail.Guardrail

	// Pre-build LLM judge if any enabled record requests engine=llm.
	var judge *llmjudge.Judge
	for _, rec := range recs {
		if !rec.IsEnabled {
			continue
		}
		if rec.Engine == "llm" {
			// Read provider + model from the llm_judge config record.
			var providerName, model string
			for _, r := range recs {
				if r.GuardrailType == "llm_judge" {
					var jcfg map[string]any
					if len(r.ConfigJSON) > 0 {
						_ = json.Unmarshal(r.ConfigJSON, &jcfg)
						if p, ok := jcfg["provider"].(string); ok {
							providerName = p
						}
						if m, ok := jcfg["model"].(string); ok {
							model = m
						}
					}
					break
				}
			}
			if providerName == "" || model == "" {
				logger.Warn("llm judge requested but provider/model not configured in llm_judge record; falling back to regex")
			} else if p, ok := registry.Get(providerName); !ok {
				logger.Warn("llm judge: provider not registered, falling back to regex", "provider", providerName)
			} else {
				judge = llmjudge.New(&providerCompleter{p: p}, model, logger)
			}
			break
		}
	}

	for _, rec := range recs {
		if !rec.IsEnabled {
			continue
		}
		action := guardrail.Action(rec.Action)

		switch rec.GuardrailType {
		case "pii":
			var cfg struct {
				Categories []string `json:"categories"`
			}
			_ = json.Unmarshal(rec.ConfigJSON, &cfg)
			d := pii.NewDetector(true, action, cfg.Categories)
			inputGuards = append(inputGuards, d)
			outputGuards = append(outputGuards, d)

		case "prompt_injection":
			if rec.Engine == "llm" && judge != nil {
				inputGuards = append(inputGuards, llmjudge.NewPromptInjectionGuard(judge, action))
			} else {
				inputGuards = append(inputGuards, injection.NewDetector(true, action))
			}

		case "content_filter":
			var cfg struct {
				Categories []string `json:"categories"`
			}
			_ = json.Unmarshal(rec.ConfigJSON, &cfg)
			if rec.Engine == "llm" && judge != nil {
				g := llmjudge.NewContentFilterGuard(judge, action)
				inputGuards = append(inputGuards, g)
				outputGuards = append(outputGuards, g)
			} else {
				f := content.NewFilter(true, action, cfg.Categories)
				inputGuards = append(inputGuards, f)
				outputGuards = append(outputGuards, f)
			}

		case "custom_keywords":
			var cfg struct {
				Blocked []string `json:"blocked"`
			}
			_ = json.Unmarshal(rec.ConfigJSON, &cfg)
			if len(cfg.Blocked) > 0 {
				f := keyword.NewFilter(true, action, cfg.Blocked)
				inputGuards = append(inputGuards, f)
				outputGuards = append(outputGuards, f)
			}

		case "llm_judge":
			// llm_judge is not a standalone guardrail; it provides config for others.
		}
	}

	if len(inputGuards) == 0 && len(outputGuards) == 0 {
		return nil
	}

	logger.Info("guardrails enabled",
		"input_count", len(inputGuards),
		"output_count", len(outputGuards))
	return guardrail.NewPipeline(inputGuards, outputGuards, logger)
}

// buildPipelineFromConfig constructs a Pipeline directly from config (used as fallback).
func buildPipelineFromConfig(cfg *config.Config, registry *provider.Registry, logger *slog.Logger) *guardrail.Pipeline {
	gc := cfg.Guardrails

	var inputGuards, outputGuards []guardrail.Guardrail

	// Build LLM judge once if any guardrail needs it.
	var judge *llmjudge.Judge
	needsLLM := (gc.PromptInjection.Enabled && gc.PromptInjection.Engine == "llm") ||
		(gc.ContentFilter.Enabled && gc.ContentFilter.Engine == "llm")
	if needsLLM {
		providerName := gc.LLMJudge.Provider
		model := gc.LLMJudge.Model
		if providerName == "" || model == "" {
			logger.Warn("llm judge requested but provider/model not configured; falling back to regex")
		} else if p, ok := registry.Get(providerName); !ok {
			logger.Warn("llm judge: provider not registered, falling back to regex", "provider", providerName)
		} else {
			judge = llmjudge.New(&providerCompleter{p: p}, model, logger)
		}
	}

	if gc.PII.Enabled {
		action := guardrail.Action(gc.PII.Action)
		d := pii.NewDetector(true, action, gc.PII.Categories)
		inputGuards = append(inputGuards, d)
		outputGuards = append(outputGuards, d)
	}

	if gc.PromptInjection.Enabled {
		action := guardrail.Action(gc.PromptInjection.Action)
		if gc.PromptInjection.Engine == "llm" && judge != nil {
			inputGuards = append(inputGuards, llmjudge.NewPromptInjectionGuard(judge, action))
		} else {
			inputGuards = append(inputGuards, injection.NewDetector(true, action))
		}
	}

	if gc.ContentFilter.Enabled {
		action := guardrail.Action(gc.ContentFilter.Action)
		if gc.ContentFilter.Engine == "llm" && judge != nil {
			g := llmjudge.NewContentFilterGuard(judge, action)
			inputGuards = append(inputGuards, g)
			outputGuards = append(outputGuards, g)
		} else {
			f := content.NewFilter(true, action, gc.ContentFilter.Categories)
			inputGuards = append(inputGuards, f)
			outputGuards = append(outputGuards, f)
		}
	}

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

// buildMCPHub creates and connects the MCP Hub from config.
// Returns nil if no servers are configured.
func buildMCPHub(ctx context.Context, cfg *config.Config, auditLog *audit.Logger, logger *slog.Logger) *mcp.Hub {
	if len(cfg.MCP.Servers) == 0 {
		return nil
	}

	hub := mcp.NewHub(logger)
	for _, sc := range cfg.MCP.Servers {
		serverCfg := mcp.ServerConfig{
			Name:    sc.Name,
			Type:    sc.Type,
			Command: sc.Command,
			Args:    sc.Args,
			Env:     sc.Env,
			URL:     sc.URL,
			APIKey:  sc.APIKey,
			Auth: mcp.ServerAuthConfig{
				Type:  sc.Auth.Type,
				Token: sc.Auth.Token,
				User:  sc.Auth.User,
			},
		}
		hub.Register(mcp.NewServer(serverCfg))
	}

	hub.Connect(ctx)
	logger.Info("mcp hub started", "servers", len(cfg.MCP.Servers))
	return hub
}

// registerMCPRoutes mounts /mcp/v1/* and /admin/mcp/* endpoints.
func registerMCPRoutes(
	r chi.Router,
	hub *mcp.Hub,
	cfg *config.Config,
	logger *slog.Logger,
	authMw func(http.Handler) http.Handler,
	auditLog *audit.Logger,
	jwtSvc *auth.JWTService,
) {
	// Collect server configs for admin handler.
	cfgs := make([]mcp.ServerConfig, 0, len(cfg.MCP.Servers))
	for _, sc := range cfg.MCP.Servers {
		cfgs = append(cfgs, mcp.ServerConfig{
			Name: sc.Name, Type: sc.Type, URL: sc.URL,
			Command: sc.Command, Args: sc.Args,
		})
	}

	auditAdapter := mcp.NewAuditAdapter(auditLog, logger)
	proxy := mcp.NewProxy(hub, logger, auditAdapter, cfg.MCP.ToolCacheTTL)
	mcpHandler := handler.NewMCPHandler(proxy, logger)
	adminMCPHandler := handler.NewAdminMCPHandler(hub, cfgs, logger)

	// Public MCP endpoints (protected by virtual key auth).
	r.Group(func(r chi.Router) {
		r.Use(authMw)
		r.Post("/mcp/v1/initialize", mcpHandler.Initialize)
		r.Post("/mcp/v1/tools/list", mcpHandler.ListTools)
		r.Post("/mcp/v1/tools/call", mcpHandler.CallTool)
		r.Post("/mcp/v1/resources/list", mcpHandler.ListResources)
		r.Post("/mcp/v1/resources/read", mcpHandler.ReadResource)
		r.Post("/mcp/v1/prompts/list", mcpHandler.ListPrompts)
		r.Post("/mcp/v1/prompts/get", mcpHandler.GetPrompt)
	})

	// Admin MCP endpoints (protected by JWT).
	adminMw := auth.AdminAuth(jwtSvc)
	r.Group(func(r chi.Router) {
		r.Use(adminMw)
		r.Get("/admin/mcp/servers", adminMCPHandler.ListServers)
		r.Post("/admin/mcp/servers", adminMCPHandler.RegisterServer)
		r.Get("/admin/mcp/servers/{name}", adminMCPHandler.GetServer)
		r.Delete("/admin/mcp/servers/{name}", adminMCPHandler.DeleteServer)
		r.Get("/admin/mcp/servers/{name}/health", adminMCPHandler.HealthCheck)
		r.Get("/admin/mcp/servers/{name}/tools", adminMCPHandler.ListServerTools)
		r.Post("/admin/mcp/policies", adminMCPHandler.SetPolicy)
	})
}

// buildMLRouter constructs the ML routing engine from config.
// Returns nil if ML routing is disabled or no quality tiers are configured.
func buildMLRouter(cfg *config.Config, pricing *cost.Calculator, tracker *health.ProviderTracker, logger *slog.Logger) *mlrouter.Router {
	if !cfg.MLRouting.Enabled || len(cfg.MLRouting.QualityTiers) == 0 {
		return nil
	}

	mode := cfg.MLRouting.Mode
	if mode == "" {
		mode = mlrouter.ModeShadow
	}

	weights := mlrouter.Weights{
		Cost:        cfg.MLRouting.Weights.Cost,
		Quality:     cfg.MLRouting.Weights.Quality,
		Latency:     cfg.MLRouting.Weights.Latency,
		Reliability: cfg.MLRouting.Weights.Reliability,
	}
	// Apply defaults if all weights are zero.
	if weights.Cost+weights.Quality+weights.Latency+weights.Reliability == 0 {
		weights = mlrouter.DefaultWeights()
	}

	tiers := make([]mlrouter.TierConfig, 0, len(cfg.MLRouting.QualityTiers))
	for _, t := range cfg.MLRouting.QualityTiers {
		models := make([]mlrouter.ModelConfig, 0, len(t.Models))
		for _, m := range t.Models {
			models = append(models, mlrouter.ModelConfig{Provider: m.Provider, Model: m.Model})
		}
		tiers = append(tiers, mlrouter.TierConfig{Name: t.Name, Models: models})
	}

	routerCfg := mlrouter.Config{
		Mode:         mode,
		Weights:      weights,
		QualityTiers: mlrouter.BuildTiers(tiers),
	}
	r := mlrouter.NewRouter(routerCfg, pricing, tracker, logger)
	logger.Info("ml routing engine enabled", "mode", mode, "tiers", len(tiers))
	return r
}

// buildResidencyRegistry converts config-file policies into a residency.Registry.
// Returns nil if data residency is disabled.
func buildResidencyRegistry(cfg *config.Config) *residency.Registry {
	if !cfg.DataResidency.Enabled || len(cfg.DataResidency.Policies) == 0 {
		return nil
	}
	policies := make([]*residency.Policy, 0, len(cfg.DataResidency.Policies))
	for _, pc := range cfg.DataResidency.Policies {
		allowed := make(map[string]residency.ProviderConstraint, len(pc.AllowedProviders))
		for _, ap := range pc.AllowedProviders {
			allowed[ap.Name] = residency.ProviderConstraint{Name: ap.Name, Region: ap.Region}
		}
		blocked := make(map[string]bool, len(pc.BlockedProviders))
		for _, b := range pc.BlockedProviders {
			blocked[b] = true
		}
		policies = append(policies, &residency.Policy{
			Name:             pc.Name,
			AllowedProviders: allowed,
			BlockedProviders: blocked,
			AllowedRegions:   pc.AllowedRegions,
		})
	}
	return residency.NewRegistry(policies)
}

// buildResidencyEnforcer creates an Enforcer if data residency is enabled.
// Returns nil when disabled so callers can safely skip residency checks.
func buildResidencyEnforcer(cfg *config.Config) *residency.Enforcer {
	registry := buildResidencyRegistry(cfg)
	if registry == nil {
		return nil
	}
	return residency.NewEnforcer(registry)
}

// syncProvidersFromDB loads enabled providers and models from DB into the
// registry and pricing table. If the DB is empty, the existing config and
// hardcoded defaults remain unchanged (no-op for fresh installs).
func syncProvidersFromDB(ctx context.Context, pool *pgxpool.Pool, reg *provider.Registry, calc *cost.Calculator, logger *slog.Logger) {
	provStore := pgstore.NewProviderStore(pool)
	modelStore := pgstore.NewModelStore(pool)

	provs, err := provStore.List(ctx)
	if err != nil {
		logger.Warn("syncProvidersFromDB: failed to list providers; using defaults", "error", err)
		return
	}
	if len(provs) == 0 {
		return // empty DB — keep existing defaults
	}

	pricing := make(map[string]cost.ModelPricing)
	for _, p := range provs {
		if !p.IsEnabled {
			continue
		}
		models, err := modelStore.ListByProvider(ctx, p.ID)
		if err != nil {
			logger.Warn("syncProvidersFromDB: failed to list models", "provider", p.Name, "error", err)
			continue
		}

		modelInfos := make([]types.ModelInfo, 0, len(models))
		for _, m := range models {
			if !m.IsEnabled {
				continue
			}
			modelInfos = append(modelInfos, types.ModelInfo{
				ID:      m.ModelID,
				Object:  "model",
				OwnedBy: p.Name,
			})
			pricing[m.ModelID] = cost.ModelPricing{
				Provider:               p.Name,
				InputPerMillionTokens:  m.InputPerMillionTokens,
				OutputPerMillionTokens: m.OutputPerMillionTokens,
			}
		}

		if ok := reg.SetProviderModels(p.Name, modelInfos); !ok {
			logger.Warn("syncProvidersFromDB: provider not found in registry (name mismatch?)",
				"db_provider_name", p.Name)
		}
	}

	if len(pricing) > 0 {
		calc.UpdatePricing(pricing)
	}
	logger.Info("syncProvidersFromDB: loaded providers from DB", "count", len(provs))
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
