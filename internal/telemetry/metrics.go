package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts completed LLM API requests.
	// Labels: provider, model, status (HTTP status code as string).
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_requests_total",
		Help: "Total number of LLM API requests completed.",
	}, []string{"provider", "model", "status"})

	// RequestDurationSeconds tracks end-to-end request latency.
	// Labels: provider, model.
	RequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_request_duration_seconds",
		Help:    "LLM API request duration in seconds.",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"provider", "model"})

	// ProviderHealth indicates whether a provider is healthy (1) or not (0).
	// Updated by the health handler based on the ProviderTracker.
	ProviderHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_provider_health",
		Help: "Provider health status: 1=ok, 0=unhealthy.",
	}, []string{"provider"})
)
