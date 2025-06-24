package config_gateway

import (
	"alt/config"
	"alt/port/config_port"
)

// ConfigGateway implements the ConfigPort interface
type ConfigGateway struct {
	config *config.Config
}

// NewConfigGateway creates a new configuration gateway
func NewConfigGateway(cfg *config.Config) *ConfigGateway {
	return &ConfigGateway{
		config: cfg,
	}
}

// GetServerPort returns the server port from configuration
func (c *ConfigGateway) GetServerPort() int {
	return c.config.Server.Port
}

// GetServerTimeouts returns server timeout configuration
func (c *ConfigGateway) GetServerTimeouts() config_port.ServerTimeouts {
	return config_port.ServerTimeouts{
		Read:  c.config.Server.ReadTimeout,
		Write: c.config.Server.WriteTimeout,
		Idle:  c.config.Server.IdleTimeout,
	}
}

// GetRateLimitConfig returns rate limiting configuration
func (c *ConfigGateway) GetRateLimitConfig() config_port.RateLimitConfig {
	return config_port.RateLimitConfig{
		ExternalAPIInterval: c.config.RateLimit.ExternalAPIInterval,
		FeedFetchLimit:      c.config.RateLimit.FeedFetchLimit,
		EnablePerHostLimit:  true, // Default to per-host limiting
	}
}

// GetDatabaseConfig returns database configuration
func (c *ConfigGateway) GetDatabaseConfig() config_port.DatabaseConfig {
	return config_port.DatabaseConfig{
		MaxConnections:    c.config.Database.MaxConnections,
		ConnectionTimeout: c.config.Database.ConnectionTimeout,
		MaxIdleTime:       300, // Default idle time (not in original config)
	}
}

// GetCacheConfig returns cache configuration
func (c *ConfigGateway) GetCacheConfig() config_port.CacheConfig {
	return config_port.CacheConfig{
		FeedCacheExpiry:   c.config.Cache.FeedCacheExpiry,
		SearchCacheExpiry: c.config.Cache.SearchCacheExpiry,
		EnableCaching:     true, // Default to enabled
	}
}

// GetLoggingConfig returns logging configuration
func (c *ConfigGateway) GetLoggingConfig() config_port.LoggingConfig {
	return config_port.LoggingConfig{
		Level:  c.config.Logging.Level,
		Format: c.config.Logging.Format,
	}
}
