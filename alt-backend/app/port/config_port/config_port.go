package config_port

import "time"

//go:generate go run go.uber.org/mock/mockgen -source=config_port.go -destination=../../mocks/mock_config_port.go

// ConfigPort defines the interface for configuration management
type ConfigPort interface {
	// Server configuration
	GetServerPort() int
	GetServerTimeouts() ServerTimeouts

	// Rate limiting configuration
	GetRateLimitConfig() RateLimitConfig

	// Database configuration
	GetDatabaseConfig() DatabaseConfig

	// Cache configuration
	GetCacheConfig() CacheConfig

	// Logging configuration
	GetLoggingConfig() LoggingConfig
}

// ServerTimeouts holds server timeout configuration
type ServerTimeouts struct {
	Read  time.Duration
	Write time.Duration
	Idle  time.Duration
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	ExternalAPIInterval time.Duration
	FeedFetchLimit      int
	EnablePerHostLimit  bool
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	MaxConnections    int
	ConnectionTimeout time.Duration
	MaxIdleTime       time.Duration
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	FeedCacheExpiry   time.Duration
	SearchCacheExpiry time.Duration
	EnableCaching     bool
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string
	Format string
}
