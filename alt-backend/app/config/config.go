package config

import (
	"time"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Database  DatabaseConfig  `json:"database"`
	RateLimit RateLimitConfig `json:"rate_limit"`
	Cache     CacheConfig     `json:"cache"`
	Logging   LoggingConfig   `json:"logging"`
	HTTP      HTTPConfig      `json:"http"`
	Auth      AuthConfig      `json:"auth"`

	// Legacy fields for backward compatibility
	Port               int           `json:"port"`
	DatabaseURL        string        `json:"database_url"`
	LogLevel           string        `json:"log_level"`
	MeilisearchURL     string        `json:"meilisearch_url"`
	RateLimitInterval  time.Duration `json:"rate_limit_interval"`
	MaxPaginationLimit int           `json:"max_pagination_limit"`
	AuthServiceURL     string        `json:"auth_service_url" env:"AUTH_SERVICE_URL" default:"http://auth-service.alt-auth.svc.cluster.local:8080"`
}

type ServerConfig struct {
	Port         int           `json:"port" env:"SERVER_PORT" default:"9000"`
	ReadTimeout  time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"300s"`
	WriteTimeout time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"300s"`
	IdleTimeout  time.Duration `json:"idle_timeout" env:"SERVER_IDLE_TIMEOUT" default:"120s"`
	SSEInterval  time.Duration `json:"sse_interval" env:"SERVER_SSE_INTERVAL" default:"5s"`
}

type RateLimitConfig struct {
	ExternalAPIInterval time.Duration `json:"external_api_interval" env:"RATE_LIMIT_EXTERNAL_API_INTERVAL" default:"5s"`
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

type AuthConfig struct {
	ServiceURL        string        `json:"service_url" env:"AUTH_SERVICE_URL" default:"http://auth-service.alt-auth.svc.cluster.local:8080"`
	Timeout           time.Duration `json:"timeout" env:"AUTH_TIMEOUT" default:"30s"`
	EnableCSRF        bool          `json:"enable_csrf" env:"AUTH_ENABLE_CSRF" default:"true"`
	RequireAuth       bool          `json:"require_auth" env:"AUTH_REQUIRE_AUTH" default:"true"`
	SessionCookieName string        `json:"session_cookie_name" env:"AUTH_SESSION_COOKIE_NAME" default:"ory_kratos_session"`
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

	return config, nil
}

// Load is an alias for NewConfig for backward compatibility
func Load() (*Config, error) {
	return NewConfig()
}
