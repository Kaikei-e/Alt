package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	RateLimit     RateLimitConfig     `json:"rate_limit"`
	Cache         CacheConfig         `json:"cache"`
	Logging       LoggingConfig       `json:"logging"`
	HTTP          HTTPConfig          `json:"http"`
	PreProcessor  PreProcessorConfig  `json:"pre_processor"`
	SearchIndexer SearchIndexerConfig `json:"search_indexer"`
	Recap         RecapConfig         `json:"recap"`
	Auth          AuthConfig          `json:"auth"`
	Rag           RAGConfig           `json:"rag"`
	AuthHub       AuthHubConfig       `json:"auth_hub"`
	MQHub         MQHubConfig         `json:"mq_hub"`
	InternalAPI   InternalAPIConfig   `json:"internal_api"`
	ImageProxy    ImageProxyConfig    `json:"image_proxy"`

	// Legacy fields for backward compatibility
	Port               int           `json:"port"`
	DatabaseURL        string        `json:"database_url"`
	LogLevel           string        `json:"log_level"`
	MeilisearchURL     string        `json:"meilisearch_url"`
	RateLimitInterval  time.Duration `json:"rate_limit_interval"`
	MaxPaginationLimit int           `json:"max_pagination_limit"`
}

type PreProcessorConfig struct {
	Enabled    bool   `json:"enabled" env:"PRE_PROCESSOR_ENABLED" default:"true"`
	URL        string `json:"url" env:"PRE_PROCESSOR_URL" default:"http://pre-processor:9200"`
	ConnectURL string `json:"connect_url" env:"PRE_PROCESSOR_CONNECT_URL" default:"http://pre-processor:9202"`
}

type SearchIndexerConfig struct {
	ConnectURL string `json:"connect_url" env:"SEARCH_INDEXER_CONNECT_URL" default:"http://search-indexer:9301"`
}

type RecapConfig struct {
	DefaultPageSize  int    `json:"default_page_size" env:"RECAP_DEFAULT_PAGE_SIZE" default:"500"`
	MaxPageSize      int    `json:"max_page_size" env:"RECAP_MAX_PAGE_SIZE" default:"2000"`
	MaxRangeDays     int    `json:"max_range_days" env:"RECAP_MAX_RANGE_DAYS" default:"8"`
	RateLimitRPS     int    `json:"rate_limit_rps" env:"RECAP_RATE_LIMIT_RPS" default:"4"`
	RateLimitBurst   int    `json:"rate_limit_burst" env:"RECAP_RATE_LIMIT_BURST" default:"8"`
	MaxArticleBytes  int    `json:"max_article_bytes" env:"RECAP_MAX_ARTICLE_BYTES" default:"2097152"`
	ClusterDraftPath string `json:"cluster_draft_path" env:"RECAP_CLUSTER_DRAFT_PATH" default:"docs/genre-reorg-draft.json"`
	WorkerURL        string `json:"worker_url" env:"RECAP_WORKER_URL" default:"http://recap-worker:9005"`
}

type RAGConfig struct {
	OrchestratorURL        string `json:"orchestrator_url" env:"RAG_ORCHESTRATOR_URL" default:"http://rag-orchestrator:9010"`
	OrchestratorConnectURL string `json:"orchestrator_connect_url" env:"RAG_ORCHESTRATOR_CONNECT_URL" default:"http://rag-orchestrator:9011"`
}

type AuthHubConfig struct {
	URL string `json:"url" env:"AUTH_HUB_URL" default:"http://auth-hub:8888"`
}

// MQHubConfig holds configuration for mq-hub event broker.
type MQHubConfig struct {
	// Enabled determines if event publishing via mq-hub is active.
	Enabled bool `json:"enabled" env:"MQHUB_ENABLED" default:"false"`
	// ConnectURL is the Connect-RPC URL for mq-hub service.
	ConnectURL string `json:"connect_url" env:"MQHUB_CONNECT_URL" default:"http://mq-hub:9500"`
}

// ImageProxyConfig holds configuration for the OGP image proxy.
type ImageProxyConfig struct {
	Enabled     bool   `json:"enabled" env:"IMAGE_PROXY_ENABLED" default:"true"`
	Secret      string `json:"secret" env:"IMAGE_PROXY_SECRET"`
	SecretFile  string `json:"-" env:"IMAGE_PROXY_SECRET_FILE"`
	CacheTTLMin int    `json:"cache_ttl_min" env:"IMAGE_PROXY_CACHE_TTL_MINUTES" default:"720"`
	MaxWidth    int    `json:"max_width" env:"IMAGE_PROXY_MAX_WIDTH" default:"600"`
	WebPQuality int    `json:"webp_quality" env:"IMAGE_PROXY_WEBP_QUALITY" default:"80"`
}

// InternalAPIConfig holds configuration for the internal service-to-service API.
type InternalAPIConfig struct {
	// ServiceSecret is the shared secret for service-to-service authentication.
	ServiceSecret     string `json:"service_secret" env:"SERVICE_SECRET"`
	ServiceSecretFile string `json:"-" env:"SERVICE_SECRET_FILE"`
}

type AuthConfig struct {
	SharedSecret           string `json:"shared_secret" env:"AUTH_SHARED_SECRET"`
	SharedSecretFile       string `json:"-" env:"AUTH_SHARED_SECRET_FILE"`
	BackendTokenSecret     string `json:"backend_token_secret" env:"BACKEND_TOKEN_SECRET"`
	BackendTokenSecretFile string `json:"-" env:"BACKEND_TOKEN_SECRET_FILE"`
	BackendTokenIssuer     string `json:"backend_token_issuer" env:"BACKEND_TOKEN_ISSUER"`
	BackendTokenAudience   string `json:"backend_token_audience" env:"BACKEND_TOKEN_AUDIENCE"`
}

type ServerConfig struct {
	Port               int           `json:"port" env:"SERVER_PORT" default:"9000"`
	ReadTimeout        time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"300s"` // Extended for LLM processing (nginx timeout 240s + margin)
	WriteTimeout       time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"300s"`
	IdleTimeout        time.Duration `json:"idle_timeout" env:"SERVER_IDLE_TIMEOUT" default:"120s"`
	SSEInterval        time.Duration `json:"sse_interval" env:"SERVER_SSE_INTERVAL" default:"5s"`
	CORSAllowedOrigins []string      `json:"cors_allowed_origins" env:"CORS_ALLOWED_ORIGINS" default:"http://localhost:3000,http://localhost:80,http://localhost:4173,https://curionoah.com"`
}

type RateLimitConfig struct {
	ExternalAPIInterval time.Duration `json:"external_api_interval" env:"RATE_LIMIT_EXTERNAL_API_INTERVAL" default:"10s"`
	FeedFetchLimit      int           `json:"feed_fetch_limit" env:"RATE_LIMIT_FEED_FETCH_LIMIT" default:"100"`

	// DOS Protection Configuration
	DOSProtection DOSProtectionConfig `json:"dos_protection"`
}

type DOSProtectionConfig struct {
	Enabled          bool                 `json:"enabled" env:"DOS_PROTECTION_ENABLED" default:"true"`
	RateLimit        int                  `json:"rate_limit" env:"DOS_PROTECTION_RATE_LIMIT" default:"100"`
	BurstLimit       int                  `json:"burst_limit" env:"DOS_PROTECTION_BURST_LIMIT" default:"200"`
	WindowSize       time.Duration        `json:"window_size" env:"DOS_PROTECTION_WINDOW_SIZE" default:"1m"`
	BlockDuration    time.Duration        `json:"block_duration" env:"DOS_PROTECTION_BLOCK_DURATION" default:"5m"`
	WhitelistedPaths []string             `json:"whitelisted_paths"`
	CircuitBreaker   CircuitBreakerConfig `json:"circuit_breaker"`
}

type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled" env:"CIRCUIT_BREAKER_ENABLED" default:"true"`
	FailureThreshold int           `json:"failure_threshold" env:"CIRCUIT_BREAKER_FAILURE_THRESHOLD" default:"10"`
	TimeoutDuration  time.Duration `json:"timeout_duration" env:"CIRCUIT_BREAKER_TIMEOUT_DURATION" default:"30s"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout" env:"CIRCUIT_BREAKER_RECOVERY_TIMEOUT" default:"60s"`
}

type DatabaseConfig struct {
	MaxConnections    int           `json:"max_connections" env:"DB_MAX_CONNECTIONS" default:"25"`
	ConnectionTimeout time.Duration `json:"connection_timeout" env:"DB_CONNECTION_TIMEOUT" default:"30s"`
}

type CacheConfig struct {
	FeedCacheExpiry   time.Duration `json:"feed_cache_expiry" env:"CACHE_FEED_EXPIRY" default:"300s"`
	SearchCacheExpiry time.Duration `json:"search_cache_expiry" env:"CACHE_SEARCH_EXPIRY" default:"900s"`
}

type LoggingConfig struct {
	Level  string `json:"level" env:"LOG_LEVEL" default:"info"`
	Format string `json:"format" env:"LOG_FORMAT" default:"json"`
}

type HTTPConfig struct {
	ClientTimeout       time.Duration `json:"client_timeout" env:"HTTP_CLIENT_TIMEOUT" default:"30s"`
	DialTimeout         time.Duration `json:"dial_timeout" env:"HTTP_DIAL_TIMEOUT" default:"10s"`
	TLSHandshakeTimeout time.Duration `json:"tls_handshake_timeout" env:"HTTP_TLS_HANDSHAKE_TIMEOUT" default:"10s"`
	IdleConnTimeout     time.Duration `json:"idle_conn_timeout" env:"HTTP_IDLE_CONN_TIMEOUT" default:"90s"`
}

// NewConfig creates a new configuration by loading from environment variables
// with fallback to default values
func NewConfig() (*Config, error) {
	config := &Config{}

	if err := loadFromEnvironment(config); err != nil {
		return nil, err
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	// Load shared secret from file if configured (Docker Secrets support)
	if config.Auth.SharedSecretFile != "" {
		content, err := os.ReadFile(config.Auth.SharedSecretFile)
		if err == nil {
			config.Auth.SharedSecret = strings.TrimSpace(string(content))
		}
		// If file read fails, we fall back to the env var value (if any) or keep it empty
	}

	// Load service secret from file if configured (Docker Secrets support)
	if config.InternalAPI.ServiceSecretFile != "" {
		content, err := os.ReadFile(config.InternalAPI.ServiceSecretFile)
		if err == nil {
			config.InternalAPI.ServiceSecret = strings.TrimSpace(string(content))
		}
	}

	// Load image proxy secret from file if configured (Docker Secrets support)
	if config.ImageProxy.SecretFile != "" {
		content, err := os.ReadFile(config.ImageProxy.SecretFile)
		if err == nil {
			config.ImageProxy.Secret = strings.TrimSpace(string(content))
		}
	}

	// Load backend token secret from file if configured (Docker Secrets support)
	if config.Auth.BackendTokenSecretFile != "" {
		content, err := os.ReadFile(config.Auth.BackendTokenSecretFile)
		if err == nil {
			config.Auth.BackendTokenSecret = strings.TrimSpace(string(content))
		}
		// If file read fails, we fall back to the env var value (if any) or keep it empty
	}

	// Set defaults for JWT issuer and audience if not provided
	if config.Auth.BackendTokenIssuer == "" {
		config.Auth.BackendTokenIssuer = "auth-hub"
	}
	if config.Auth.BackendTokenAudience == "" {
		config.Auth.BackendTokenAudience = "alt-backend"
	}

	// Validate auth configuration after secrets are loaded
	// This ensures fail-fast behavior for misconfigured production deployments
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}
	if err := validateAuthConfig(&config.Auth, env); err != nil {
		return nil, fmt.Errorf("auth config validation failed: %w", err)
	}

	return config, nil
}

// Load is an alias for NewConfig for backward compatibility
func Load() (*Config, error) {
	return NewConfig()
}
