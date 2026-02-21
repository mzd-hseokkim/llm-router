package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/budget"
	"github.com/llm-router/gateway/internal/config"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/crypto"
	"github.com/llm-router/gateway/internal/gateway/circuitbreaker"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/handler"
	"github.com/llm-router/gateway/internal/gateway/router"
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
	"time"
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

	// Build named fallback chains from config.
	chains := buildFallbackChains(cfg)

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

	// --- Provider health tracker ---
	tracker := health.NewProviderTracker()

	// --- Health checker ---
	checker := health.NewChecker(pool, redisClient, version)
	healthHandler := handler.NewHealthHandler(checker, tracker, registry)

	// --- HTTP server ---
	srv := server.New(cfg.Server, logger)
	r := srv.Router()

	// /v1 routes — protected by virtual key auth
	router.Setup(r, registry, fr, chains, logger, authMw.Middleware, logWriter, tracker,
		rateLimiter, budgetMgr, costCalc)

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
	registerAdminRoutes(r, keyStore, keyCache, km, cipher, pool, logStore, cb, cfg, logger,
		rateLimiter, budgetMgr, budgetStore)

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
func buildFallbackChains(cfg *config.Config) []fallback.Chain {
	chains := make([]fallback.Chain, 0, len(cfg.Routing.FallbackChains))
	for _, fc := range cfg.Routing.FallbackChains {
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
) {
	adminMw := auth.AdminAuth(cfg.Gateway.MasterKey)

	r.Group(func(r chi.Router) {
		r.Use(adminMw)

		// Virtual key CRUD
		vkHandler := handler.NewAdminKeysHandler(store, cache, logger)
		r.Post("/admin/keys", vkHandler.Create)
		r.Get("/admin/keys", vkHandler.List)
		r.Get("/admin/keys/{id}", vkHandler.Get)
		r.Patch("/admin/keys/{id}", vkHandler.Update)
		r.Delete("/admin/keys/{id}", vkHandler.Deactivate)

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
	})
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
