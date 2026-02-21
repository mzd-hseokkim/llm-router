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
	Server    ServerConfig    `koanf:"server"`
	Database  DatabaseConfig  `koanf:"database"`
	Redis     RedisConfig     `koanf:"redis"`
	Log       LogConfig       `koanf:"log"`
	Providers ProvidersConfig `koanf:"providers"`
	Gateway   GatewayConfig   `koanf:"gateway"`
	Routing   RoutingConfig   `koanf:"routing"`
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
	OpenAI    ProviderConfig       `koanf:"openai"`
	Anthropic ProviderConfig       `koanf:"anthropic"`
	Gemini    ProviderConfig       `koanf:"gemini"`
	Azure     AzureProviderConfig  `koanf:"azure"`
	Mistral   ProviderConfig       `koanf:"mistral"`
	Cohere    ProviderConfig       `koanf:"cohere"`
	Bedrock   BedrockProviderConfig `koanf:"bedrock"`
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
	Port         int           `koanf:"port"`
	ReadTimeout  time.Duration `koanf:"read_timeout"`
	WriteTimeout time.Duration `koanf:"write_timeout"`
}

type DatabaseConfig struct {
	URL            string `koanf:"url"`
	MaxConnections int    `koanf:"max_connections"`
}

type RedisConfig struct {
	Addr string `koanf:"addr"`
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
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "localhost:6379"
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
}
