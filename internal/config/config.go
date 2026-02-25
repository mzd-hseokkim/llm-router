package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Server        ServerConfig        `koanf:"server"`
	Database      DatabaseConfig      `koanf:"database"`
	Redis         RedisConfig         `koanf:"redis"`
	Log           LogConfig           `koanf:"log"`
	Providers     ProvidersConfig     `koanf:"providers"`
	Gateway       GatewayConfig       `koanf:"gateway"`
	Routing       RoutingConfig       `koanf:"routing"`
	Auth          AuthConfig          `koanf:"auth"`
	Guardrails    GuardrailConfig     `koanf:"guardrails"`
	Cache         CacheConfig         `koanf:"cache"`
	Alerting      AlertingConfig      `koanf:"alerting"`
	MCP           MCPConfig           `koanf:"mcp"`
	DataResidency DataResidencyConfig `koanf:"data_residency"`
	MLRouting     MLRoutingConfig     `koanf:"ml_routing"`
}

// DataResidencyConfig holds data residency policy configuration.
type DataResidencyConfig struct {
	Enabled  bool                    `koanf:"enabled"`
	Policies []ResidencyPolicyConfig `koanf:"policies"`
}

// ResidencyPolicyConfig defines one named residency policy.
type ResidencyPolicyConfig struct {
	Name             string                   `koanf:"name"`
	AllowedProviders []AllowedProviderConfig  `koanf:"allowed_providers"`
	BlockedProviders []string                 `koanf:"blocked_providers"`
	AllowedRegions   []string                 `koanf:"allowed_regions"`
}

// AllowedProviderConfig allows a provider, optionally constrained to a region.
type AllowedProviderConfig struct {
	Name   string `koanf:"name"`
	Region string `koanf:"region"` // optional; informational only
}

// MLRoutingConfig holds ML-based intelligent routing configuration.
type MLRoutingConfig struct {
	Enabled bool   `koanf:"enabled"`
	Mode    string `koanf:"mode"` // "shadow" | "live"

	Weights      MLRoutingWeights      `koanf:"weights"`
	QualityTiers []MLQualityTierConfig `koanf:"quality_tiers"`
}

// MLRoutingWeights defines the relative weights for the routing score.
type MLRoutingWeights struct {
	Cost        float64 `koanf:"cost"`
	Quality     float64 `koanf:"quality"`
	Latency     float64 `koanf:"latency"`
	Reliability float64 `koanf:"reliability"`
}

// MLQualityTierConfig maps a tier name to provider+model entries.
type MLQualityTierConfig struct {
	Name    string           `koanf:"name"` // economy | medium | premium
	Models  []MLModelConfig  `koanf:"models"`
}

// MLModelConfig is a provider+model pair in a quality tier.
type MLModelConfig struct {
	Provider string `koanf:"provider"`
	Model    string `koanf:"model"`
}

// MCPConfig holds Model Context Protocol Gateway configuration.
type MCPConfig struct {
	// Enabled controls whether the MCP Hub is started.
	Enabled bool `koanf:"enabled"`

	// Servers is the list of upstream MCP servers to connect to.
	Servers []MCPServerConfig `koanf:"servers"`

	// ToolCacheTTL is how long to cache idempotent tool results.
	// Set to 0 to disable caching.
	ToolCacheTTL time.Duration `koanf:"tool_cache_ttl"`
}

// MCPServerConfig describes one upstream MCP server.
type MCPServerConfig struct {
	Name    string            `koanf:"name"`
	Type    string            `koanf:"type"`
	Command string            `koanf:"command"`
	Args    []string          `koanf:"args"`
	Env     map[string]string `koanf:"env"`
	URL     string            `koanf:"url"`
	APIKey  string            `koanf:"api_key"`
	Auth    MCPServerAuth     `koanf:"auth"`
}

// MCPServerAuth holds auth details for remote MCP servers.
type MCPServerAuth struct {
	Type  string `koanf:"type"`
	Token string `koanf:"token"`
	User  string `koanf:"user"`
}

// AuthConfig holds OAuth / SSO provider configuration.
type AuthConfig struct {
	SessionTTL time.Duration       `koanf:"session_ttl"`
	Providers  []OAuthProviderConfig `koanf:"providers"`
}

// OAuthProviderConfig configures a single OAuth / OIDC provider.
type OAuthProviderConfig struct {
	Name         string            `koanf:"name"`           // "google", "github", "okta"
	Type         string            `koanf:"type"`           // "oauth2" | "oidc"
	ClientID     string            `koanf:"client_id"`
	ClientSecret string            `koanf:"client_secret"`
	IssuerURL    string            `koanf:"issuer_url"`     // OIDC only
	Scopes       []string          `koanf:"scopes"`
	// GroupRoleMapping maps IdP group names to Gateway roles (OIDC only).
	GroupRoleMapping map[string]string `koanf:"group_role_mapping"`
}

// IsEnabled returns true if the provider has the minimum required credentials set.
func (p OAuthProviderConfig) IsEnabled() bool {
	return p.ClientID != "" && p.ClientSecret != ""
}

type GatewayConfig struct {
	// MasterKey is the static key required for Admin API access.
	// Set via env var MASTER_KEY or config file gateway.master_key.
	MasterKey string `koanf:"master_key"`

	// EncryptionKey is the 32-byte base64-encoded key used to encrypt provider API keys in the DB.
	// Set via env var ENCRYPTION_KEY.
	EncryptionKey string `koanf:"encryption_key"`
}

type ProvidersConfig struct {
	OpenAI     ProviderConfig          `koanf:"openai"`
	Anthropic  ProviderConfig          `koanf:"anthropic"`
	Gemini     ProviderConfig          `koanf:"gemini"`
	Grok       ProviderConfig          `koanf:"grok"`
	Azure      AzureProviderConfig     `koanf:"azure"`
	Mistral    ProviderConfig          `koanf:"mistral"`
	Cohere     ProviderConfig          `koanf:"cohere"`
	Bedrock    BedrockProviderConfig   `koanf:"bedrock"`
	SelfHosted []SelfHostedConfig      `koanf:"self_hosted"`
}

// SelfHostedConfig configures a self-hosted LLM inference server.
type SelfHostedConfig struct {
	// Name is the provider identifier used in model prefixes, e.g. "ollama_local".
	Name    string                    `koanf:"name"`
	// Engine is one of: ollama, vllm, tgi, lmstudio.
	Engine  string                    `koanf:"engine"`
	BaseURL string                    `koanf:"base_url"`
	Models  []SelfHostedModelConfig   `koanf:"models"`
	// InitialLoadTimeout is the timeout for the first request (model loading may take minutes).
	InitialLoadTimeout string         `koanf:"initial_load_timeout"`
}

// SelfHostedModelConfig describes one model exposed by a self-hosted engine.
type SelfHostedModelConfig struct {
	// ID is the Gateway model identifier, e.g. "ollama/llama3.2:3b".
	ID          string  `koanf:"id"`
	// ModelName is the name used in requests to the inference server.
	ModelName   string  `koanf:"model_name"`
	ContextWindow int   `koanf:"context_window"`
	// InputCostPerMillion and OutputCostPerMillion are optional cost overrides (USD).
	InputCostPerMillion  float64 `koanf:"input_cost_per_million"`
	OutputCostPerMillion float64 `koanf:"output_cost_per_million"`
}

type ProviderConfig struct {
	APIKey  string `koanf:"api_key"`
	BaseURL string `koanf:"base_url"`
}

// AzureProviderConfig holds Azure OpenAI configuration.
type AzureProviderConfig struct {
	APIKey       string                   `koanf:"api_key"`
	ResourceName string                   `koanf:"resource_name"`
	APIVersion   string                   `koanf:"api_version"`
	BaseURL      string                   `koanf:"base_url"` // optional override
	Deployments  []AzureDeploymentConfig  `koanf:"deployments"`
}

// AzureDeploymentConfig maps a deployment ID to a model name.
type AzureDeploymentConfig struct {
	ID    string `koanf:"id"`
	Model string `koanf:"model"`
}

// BedrockProviderConfig holds AWS Bedrock configuration.
type BedrockProviderConfig struct {
	Region          string `koanf:"region"`
	AccessKeyID     string `koanf:"access_key_id"`
	SecretAccessKey string `koanf:"secret_access_key"`
	SessionToken    string `koanf:"session_token"`
}

// RoutingConfig holds circuit breaker and fallback chain configuration.
type RoutingConfig struct {
	CircuitBreaker CircuitBreakerConfig `koanf:"circuit_breaker"`
	FallbackChains []FallbackChainConfig `koanf:"fallback_chains"`
}

// CircuitBreakerConfig holds circuit breaker thresholds.
type CircuitBreakerConfig struct {
	FailureThreshold int           `koanf:"failure_threshold"`
	SuccessThreshold int           `koanf:"success_threshold"`
	OpenTimeout      time.Duration `koanf:"open_timeout"`
}

// FallbackChainConfig defines a named fallback chain.
type FallbackChainConfig struct {
	Name    string               `koanf:"name"`
	Targets []FallbackTargetConfig `koanf:"chain"`
}

// FallbackTargetConfig is one entry in a fallback chain.
type FallbackTargetConfig struct {
	Provider string `koanf:"provider"`
	Model    string `koanf:"model"`
	Weight   int    `koanf:"weight"`
}

type ServerConfig struct {
	Port              int           `koanf:"port"`
	ReadTimeout       time.Duration `koanf:"read_timeout"`
	WriteTimeout      time.Duration `koanf:"write_timeout"`
	IdleTimeout       time.Duration `koanf:"idle_timeout"`
	ReadHeaderTimeout time.Duration `koanf:"read_header_timeout"`
}

type DatabaseConfig struct {
	URL            string `koanf:"url"`
	MaxConnections int    `koanf:"max_connections"`
}

type RedisConfig struct {
	Addr         string `koanf:"addr"`
	PoolSize     int    `koanf:"pool_size"`
	MinIdleConns int    `koanf:"min_idle_conns"`
}

type LogConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// 1. Load from YAML file (optional)
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		// Only fail if the file exists but can't be parsed
		// Missing file is acceptable (use defaults + env)
	}

	// 2. Override with environment variables (LLM_ROUTER_ prefix)
	if err := k.Load(env.Provider("LLM_ROUTER_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "LLM_ROUTER_")), "_", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	applyDefaults(cfg)
	return cfg, nil
}

// GuardrailConfig holds guardrail policy configuration.
type GuardrailConfig struct {
	LLMJudge        LLMJudgeConfig                  `koanf:"llm_judge"`
	PII              PIIGuardrailConfig              `koanf:"pii"`
	PromptInjection  PromptInjectionGuardrailConfig  `koanf:"prompt_injection"`
	ContentFilter    ContentFilterGuardrailConfig    `koanf:"content_filter"`
	CustomKeywords   CustomKeywordsGuardrailConfig   `koanf:"custom_keywords"`
}

// LLMJudgeConfig configures the LLM used for AI-based guardrail decisions.
type LLMJudgeConfig struct {
	// Provider is the registered provider name to use (e.g. "anthropic", "openai").
	Provider string `koanf:"provider"`
	// Model is the model ID within that provider.
	Model string `koanf:"model"`
}

type PIIGuardrailConfig struct {
	Enabled    bool     `koanf:"enabled"`
	Action     string   `koanf:"action"`     // block | mask | log_only
	Categories []string `koanf:"categories"` // credit_card, ssn, email, phone_us, ip_address, korean_rrn
}

type PromptInjectionGuardrailConfig struct {
	Enabled bool   `koanf:"enabled"`
	Action  string `koanf:"action"` // block
	Engine  string `koanf:"engine"` // regex | llm (default: regex)
}

type ContentFilterGuardrailConfig struct {
	Enabled    bool     `koanf:"enabled"`
	Action     string   `koanf:"action"`
	Engine     string   `koanf:"engine"`     // regex | llm (default: regex)
	Categories []string `koanf:"categories"` // hate, violence, sexual (regex engine only)
}

type CustomKeywordsGuardrailConfig struct {
	Enabled  bool     `koanf:"enabled"`
	Action   string   `koanf:"action"`
	Blocked  []string `koanf:"blocked"`
}

// CacheConfig holds exact-match cache configuration.
// Only temperature=0 requests are cached (non-deterministic responses are never cached).
type CacheConfig struct {
	ExactMatch ExactCacheConfig `koanf:"exact_match"`
}

type ExactCacheConfig struct {
	Enabled         bool          `koanf:"enabled"`
	DefaultTTL      time.Duration `koanf:"default_ttl"`
	MaxTTL          time.Duration `koanf:"max_ttl"`
	MaxResponseSize int64         `koanf:"max_response_size"`
}

// AlertingConfig holds alerting channel and routing configuration.
type AlertingConfig struct {
	Channels []AlertChannelConfig  `koanf:"channels"`
	Routing  []AlertRoutingConfig  `koanf:"routing"`
}

// AlertChannelConfig defines one notification channel.
type AlertChannelConfig struct {
	Name       string            `koanf:"name"`
	Type       string            `koanf:"type"`         // slack | email | webhook
	WebhookURL string            `koanf:"webhook_url"`  // Slack or generic webhook
	URL        string            `koanf:"url"`          // generic webhook
	Method     string            `koanf:"method"`       // POST
	Headers    map[string]string `koanf:"headers"`
	Retry      int               `koanf:"retry"`
	// Email-specific
	SMTPHost string   `koanf:"smtp_host"`
	SMTPPort int      `koanf:"smtp_port"`
	From     string   `koanf:"from"`
	To       []string `koanf:"to"`
}

// AlertRoutingConfig maps event types and severity to channels.
type AlertRoutingConfig struct {
	Events   []string `koanf:"events"`
	Severity string   `koanf:"severity"`
	Channels []string `koanf:"channels"`
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 60 * time.Second
	}
	if cfg.Database.MaxConnections == 0 {
		cfg.Database.MaxConnections = 20
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 60 * time.Second
	}
	if cfg.Server.ReadHeaderTimeout == 0 {
		cfg.Server.ReadHeaderTimeout = 10 * time.Second
	}
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "localhost:6379"
	}
	if cfg.Redis.PoolSize == 0 {
		cfg.Redis.PoolSize = 50
	}
	if cfg.Redis.MinIdleConns == 0 {
		cfg.Redis.MinIdleConns = 10
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
	if cfg.Routing.CircuitBreaker.FailureThreshold == 0 {
		cfg.Routing.CircuitBreaker.FailureThreshold = 5
	}
	if cfg.Routing.CircuitBreaker.SuccessThreshold == 0 {
		cfg.Routing.CircuitBreaker.SuccessThreshold = 2
	}
	if cfg.Routing.CircuitBreaker.OpenTimeout == 0 {
		cfg.Routing.CircuitBreaker.OpenTimeout = 60 * time.Second
	}
	if cfg.Providers.Azure.APIVersion == "" {
		cfg.Providers.Azure.APIVersion = "2024-02-01"
	}
	if cfg.Providers.Bedrock.Region == "" {
		cfg.Providers.Bedrock.Region = "us-east-1"
	}
	if cfg.Auth.SessionTTL == 0 {
		cfg.Auth.SessionTTL = 24 * time.Hour
	}
	if cfg.Guardrails.PII.Action == "" {
		cfg.Guardrails.PII.Action = "mask"
	}
	if cfg.Guardrails.PromptInjection.Action == "" {
		cfg.Guardrails.PromptInjection.Action = "block"
	}
	if cfg.Guardrails.ContentFilter.Action == "" {
		cfg.Guardrails.ContentFilter.Action = "block"
	}
	if cfg.Guardrails.CustomKeywords.Action == "" {
		cfg.Guardrails.CustomKeywords.Action = "block"
	}
	if cfg.Cache.ExactMatch.DefaultTTL == 0 {
		cfg.Cache.ExactMatch.DefaultTTL = 24 * time.Hour
	}
	if cfg.Cache.ExactMatch.MaxTTL == 0 {
		cfg.Cache.ExactMatch.MaxTTL = 7 * 24 * time.Hour
	}
	if cfg.Cache.ExactMatch.MaxResponseSize == 0 {
		cfg.Cache.ExactMatch.MaxResponseSize = 1 << 20 // 1MB
	}
}
