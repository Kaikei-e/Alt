package config

import (
	"fmt"
	"strings"
	"time"
)

// validateConfig validates the loaded configuration values
func validateConfig(config *Config) error {
	if err := validateServerConfig(&config.Server); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if err := validateDatabaseConfig(&config.Database); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	if err := validateRateLimitConfig(&config.RateLimit); err != nil {
		return fmt.Errorf("rate limit config validation failed: %w", err)
	}

	if err := validateCacheConfig(&config.Cache); err != nil {
		return fmt.Errorf("cache config validation failed: %w", err)
	}

	if err := validateLoggingConfig(&config.Logging); err != nil {
		return fmt.Errorf("logging config validation failed: %w", err)
	}

	if err := validateHTTPConfig(&config.HTTP); err != nil {
		return fmt.Errorf("HTTP config validation failed: %w", err)
	}

	if err := validateRecapConfig(&config.Recap); err != nil {
		return fmt.Errorf("recap config validation failed: %w", err)
	}

	return nil
}

func validateServerConfig(config *ServerConfig) error {
	// Validate port range
	if config.Port < 1 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", config.Port)
	}

	// Validate timeout values
	if config.ReadTimeout <= 0 {
		return fmt.Errorf("timeout values must be positive, got ReadTimeout: %v", config.ReadTimeout)
	}

	if config.WriteTimeout <= 0 {
		return fmt.Errorf("timeout values must be positive, got WriteTimeout: %v", config.WriteTimeout)
	}

	if config.IdleTimeout <= 0 {
		return fmt.Errorf("timeout values must be positive, got IdleTimeout: %v", config.IdleTimeout)
	}

	return nil
}

func validateDatabaseConfig(config *DatabaseConfig) error {
	// Validate max connections
	if config.MaxConnections < 1 {
		return fmt.Errorf("max connections must be at least 1, got %d", config.MaxConnections)
	}

	// Validate connection timeout
	if config.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection timeout must be positive, got %v", config.ConnectionTimeout)
	}

	return nil
}

func validateRateLimitConfig(config *RateLimitConfig) error {
	// Validate external API interval (must be at least 1 second as per CLAUDE.md)
	if config.ExternalAPIInterval < time.Second {
		return fmt.Errorf("external API interval must be at least 1 second, got %v", config.ExternalAPIInterval)
	}

	// Validate feed fetch limit
	if config.FeedFetchLimit < 1 {
		return fmt.Errorf("feed fetch limit must be at least 1, got %d", config.FeedFetchLimit)
	}

	// Validate DOS protection configuration
	if err := validateDOSProtectionConfig(&config.DOSProtection); err != nil {
		return fmt.Errorf("DOS protection config validation failed: %w", err)
	}

	return nil
}

func validateCacheConfig(config *CacheConfig) error {
	// Validate cache expiry values
	if config.FeedCacheExpiry <= 0 {
		return fmt.Errorf("feed cache expiry must be positive, got %v", config.FeedCacheExpiry)
	}

	if config.SearchCacheExpiry <= 0 {
		return fmt.Errorf("search cache expiry must be positive, got %v", config.SearchCacheExpiry)
	}

	return nil
}

func validateLoggingConfig(config *LoggingConfig) error {
	// Validate log level
	validLevels := []string{"debug", "info", "warn", "error"}
	level := strings.ToLower(config.Level)

	valid := false
	for _, validLevel := range validLevels {
		if level == validLevel {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("log level must be one of: %s, got %s",
			strings.Join(validLevels, ", "), config.Level)
	}

	// Validate log format
	validFormats := []string{"json", "text"}
	format := strings.ToLower(config.Format)

	valid = false
	for _, validFormat := range validFormats {
		if format == validFormat {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("log format must be one of: %s, got %s",
			strings.Join(validFormats, ", "), config.Format)
	}

	return nil
}

func validateHTTPConfig(config *HTTPConfig) error {
	// Validate HTTP timeouts
	if config.ClientTimeout <= 0 {
		return fmt.Errorf("client timeout must be positive, got %v", config.ClientTimeout)
	}

	if config.DialTimeout <= 0 {
		return fmt.Errorf("dial timeout must be positive, got %v", config.DialTimeout)
	}

	if config.TLSHandshakeTimeout <= 0 {
		return fmt.Errorf("TLS handshake timeout must be positive, got %v", config.TLSHandshakeTimeout)
	}

	if config.IdleConnTimeout <= 0 {
		return fmt.Errorf("idle connection timeout must be positive, got %v", config.IdleConnTimeout)
	}

	return nil
}

func validateDOSProtectionConfig(config *DOSProtectionConfig) error {
	// Skip validation if DOS protection is disabled
	if !config.Enabled {
		return nil
	}

	// Validate rate limit
	if config.RateLimit <= 0 {
		return fmt.Errorf("rate limit must be greater than 0, got %d", config.RateLimit)
	}

	// Validate burst limit
	if config.BurstLimit <= 0 {
		return fmt.Errorf("burst limit must be greater than 0, got %d", config.BurstLimit)
	}

	// Validate that burst limit is >= rate limit
	if config.BurstLimit < config.RateLimit {
		return fmt.Errorf("burst limit must be >= rate limit, got burst: %d, rate: %d",
			config.BurstLimit, config.RateLimit)
	}

	// Validate window size
	if config.WindowSize <= 0 {
		return fmt.Errorf("window size must be positive, got %v", config.WindowSize)
	}

	// Validate block duration
	if config.BlockDuration <= 0 {
		return fmt.Errorf("block duration must be positive, got %v", config.BlockDuration)
	}

	// Validate circuit breaker configuration
	if err := validateCircuitBreakerConfig(&config.CircuitBreaker); err != nil {
		return fmt.Errorf("circuit breaker config validation failed: %w", err)
	}

	return nil
}

func validateRecapConfig(config *RecapConfig) error {
	if config.DefaultPageSize <= 0 {
		return fmt.Errorf("default page size must be positive, got %d", config.DefaultPageSize)
	}
	if config.MaxPageSize <= 0 {
		return fmt.Errorf("max page size must be positive, got %d", config.MaxPageSize)
	}
	if config.MaxPageSize < config.DefaultPageSize {
		return fmt.Errorf("max page size must be >= default page size (got max=%d, default=%d)", config.MaxPageSize, config.DefaultPageSize)
	}
	if config.MaxRangeDays <= 0 {
		return fmt.Errorf("max range days must be positive, got %d", config.MaxRangeDays)
	}
	if config.RateLimitRPS <= 0 {
		return fmt.Errorf("rate limit RPS must be positive, got %d", config.RateLimitRPS)
	}
	if config.RateLimitBurst <= 0 {
		return fmt.Errorf("rate limit burst must be positive, got %d", config.RateLimitBurst)
	}
	if config.RateLimitBurst < config.RateLimitRPS {
		return fmt.Errorf("rate limit burst must be >= RPS (burst=%d, rps=%d)", config.RateLimitBurst, config.RateLimitRPS)
	}
	if config.MaxArticleBytes <= 0 {
		return fmt.Errorf("max article bytes must be positive, got %d", config.MaxArticleBytes)
	}
	if strings.TrimSpace(config.ClusterDraftPath) == "" {
		return fmt.Errorf("cluster draft path must be provided")
	}
	return nil
}

func validateCircuitBreakerConfig(config *CircuitBreakerConfig) error {
	// Skip validation if circuit breaker is disabled
	if !config.Enabled {
		return nil
	}

	// Validate failure threshold
	if config.FailureThreshold <= 0 {
		return fmt.Errorf("failure threshold must be greater than 0, got %d", config.FailureThreshold)
	}

	// Validate timeout duration
	if config.TimeoutDuration <= 0 {
		return fmt.Errorf("timeout duration must be positive, got %v", config.TimeoutDuration)
	}

	// Validate recovery timeout
	if config.RecoveryTimeout <= 0 {
		return fmt.Errorf("recovery timeout must be positive, got %v", config.RecoveryTimeout)
	}

	// Validate that recovery timeout is reasonable compared to timeout duration
	if config.RecoveryTimeout < config.TimeoutDuration {
		return fmt.Errorf("recovery timeout should be >= timeout duration, got recovery: %v, timeout: %v",
			config.RecoveryTimeout, config.TimeoutDuration)
	}

	return nil
}
