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
}

type ServerConfig struct {
	Port         int           `json:"port" env:"SERVER_PORT" default:"9000"`
	ReadTimeout  time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"30s"`
	WriteTimeout time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"30s"`
	IdleTimeout  time.Duration `json:"idle_timeout" env:"SERVER_IDLE_TIMEOUT" default:"120s"`
}

type RateLimitConfig struct {
	ExternalAPIInterval time.Duration `json:"external_api_interval" env:"RATE_LIMIT_EXTERNAL_API_INTERVAL" default:"5s"`
	FeedFetchLimit      int           `json:"feed_fetch_limit" env:"RATE_LIMIT_FEED_FETCH_LIMIT" default:"100"`
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
	
	return config, nil
}