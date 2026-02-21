package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- Request metrics ---

	// RequestsTotal counts completed LLM API requests.
	// Labels: provider, model, status (HTTP status code), cache_result (hit|semantic_hit|miss).
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_requests_total",
		Help: "Total number of LLM API requests completed.",
	}, []string{"provider", "model", "status", "cache_result"})

	// RequestDurationSeconds tracks end-to-end request latency.
	RequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_request_duration_seconds",
		Help:    "LLM API request duration in seconds.",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"provider", "model"})

	// StreamingTTFTSeconds tracks time-to-first-token for streaming requests.
	StreamingTTFTSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_streaming_ttft_seconds",
		Help:    "Time to first token for streaming LLM requests (seconds).",
		Buckets: []float64{.05, .1, .25, .5, 1, 2, 5, 10},
	}, []string{"provider", "model"})

	// --- Token / cost metrics ---

	// TokensTotal counts tokens processed.
	// Labels: provider, model, type (input|output).
	TokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_tokens_total",
		Help: "Total tokens processed.",
	}, []string{"provider", "model", "type"})

	// CostUSDTotal tracks cumulative request cost in USD.
	// Labels: provider, model, team_id.
	CostUSDTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_cost_usd_total",
		Help: "Cumulative LLM request cost in USD.",
	}, []string{"provider", "model", "team_id"})

	// --- Provider metrics ---

	// ProviderRequestsTotal counts requests per provider.
	// Labels: provider, status (success|error).
	ProviderRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_provider_requests_total",
		Help: "Total requests sent to each LLM provider.",
	}, []string{"provider", "status"})

	// ProviderLatencySeconds tracks provider response latency.
	ProviderLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_provider_latency_seconds",
		Help:    "LLM provider response latency in seconds.",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"provider"})

	// ProviderHealth indicates whether a provider is healthy (1) or not (0).
	ProviderHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_provider_health",
		Help: "Provider health status: 1=ok, 0=unhealthy.",
	}, []string{"provider"})

	// --- Fallback / circuit breaker metrics ---

	// FallbackTotal counts fallback events.
	// Labels: from_provider, to_provider, reason.
	FallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_fallback_total",
		Help: "Number of provider fallback events.",
	}, []string{"from_provider", "to_provider", "reason"})

	// CircuitBreakerState tracks circuit breaker state per provider.
	// Values: 0=closed (healthy), 1=open (blocking), 2=half_open (testing).
	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_circuit_breaker_state",
		Help: "Circuit breaker state per provider: 0=closed, 1=open, 2=half_open.",
	}, []string{"provider"})

	// --- Cache metrics ---

	// CacheRequestsTotal counts cache lookups.
	// Labels: type (exact), result (hit|miss).
	CacheRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_cache_requests_total",
		Help: "Total cache lookup requests.",
	}, []string{"type", "result"})

	// CacheHitRatio tracks the rolling cache hit ratio.
	// Labels: type (exact).
	CacheHitRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_cache_hit_ratio",
		Help: "Rolling cache hit ratio (approximate).",
	}, []string{"type"})

	// --- Rate limit metrics ---

	// RateLimitExceededTotal counts rate-limit rejections.
	// Labels: entity_type (key|team|org), limit_type (rpm|tpm).
	RateLimitExceededTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_rate_limit_exceeded_total",
		Help: "Total requests rejected due to rate limiting.",
	}, []string{"entity_type", "limit_type"})

	// --- System metrics ---

	// ActiveConnections is the current number of active HTTP connections.
	ActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gateway_active_connections",
		Help: "Current number of active HTTP connections.",
	})

	// UptimeSeconds tracks gateway uptime.
	UptimeSeconds = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_uptime_seconds_total",
		Help: "Total seconds the gateway has been running.",
	})

	// Goroutines tracks the current number of Go routines.
	Goroutines = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "gateway_goroutines",
		Help: "Current number of goroutines.",
	}, func() float64 {
		// imported lazily to avoid import cycle
		return float64(currentGoroutines())
	})
)
