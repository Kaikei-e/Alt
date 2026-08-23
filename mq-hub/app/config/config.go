// Package config provides configuration management for mq-hub.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for mq-hub.
type Config struct {
	// RedisURL is the Redis connection URL.
	RedisURL string
	// ConnectPort is the port for the Connect-RPC server.
	ConnectPort int
	// LogLevel is the logging level.
	LogLevel string
	// RedisPoolSize is the maximum number of Redis connections.
	RedisPoolSize int
	// MaxBatchSize is the maximum number of events in a batch.
	MaxBatchSize int
	// StreamMaxLen is the approximate max length for Redis Streams trimming via XADD MAXLEN ~.
	// 0 means no trimming.
	StreamMaxLen int64
	// StreamHardMaxLen is the ceiling enforced by the periodic trim pass, which
	// runs independently of publishing. Set it well above StreamMaxLen: it
	// ignores consumer references, so it firing at all means the publish-time
	// trim failed to keep up. 0 disables the pass.
	StreamHardMaxLen int64
	// StreamTrimInterval is how often the periodic trim pass runs. It also
	// paces the reply-stream safety-net sweep.
	StreamTrimInterval time.Duration
	// ReplyStreamSweepEnabled controls the periodic safety-net sweep that
	// re-applies a TTL to temporary request-reply streams a late worker reply
	// may have recreated without one. Defaults to true; set
	// REPLY_STREAM_SWEEP_ENABLED=false to disable it as an explicit, logged
	// choice rather than something inferred from an unset variable.
	ReplyStreamSweepEnabled bool
}

// NewConfig creates a new Config from environment variables. It fails fast
// (returns an error) if any numeric env var is set but not parseable,
// instead of silently treating it as 0.
func NewConfig() (*Config, error) {
	port, err := strconv.Atoi(getEnvOrDefault("CONNECT_PORT", "9500"))
	if err != nil {
		return nil, fmt.Errorf("parse CONNECT_PORT: %w", err)
	}
	poolSize, err := strconv.Atoi(getEnvOrDefault("REDIS_POOL_SIZE", "10"))
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_POOL_SIZE: %w", err)
	}
	maxBatchSize, err := strconv.Atoi(getEnvOrDefault("MAX_BATCH_SIZE", "1000"))
	if err != nil {
		return nil, fmt.Errorf("parse MAX_BATCH_SIZE: %w", err)
	}
	streamMaxLen, err := strconv.ParseInt(getEnvOrDefault("STREAM_MAX_LEN", "10000"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse STREAM_MAX_LEN: %w", err)
	}
	streamHardMaxLen, err := strconv.ParseInt(getEnvOrDefault("STREAM_HARD_MAX_LEN", "50000"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse STREAM_HARD_MAX_LEN: %w", err)
	}
	trimIntervalSeconds, err := strconv.Atoi(getEnvOrDefault("STREAM_TRIM_INTERVAL_SECONDS", "60"))
	if err != nil {
		return nil, fmt.Errorf("parse STREAM_TRIM_INTERVAL_SECONDS: %w", err)
	}
	if trimIntervalSeconds <= 0 {
		return nil, fmt.Errorf("STREAM_TRIM_INTERVAL_SECONDS must be positive, got %d", trimIntervalSeconds)
	}
	replyStreamSweepEnabled, err := strconv.ParseBool(getEnvOrDefault("REPLY_STREAM_SWEEP_ENABLED", "true"))
	if err != nil {
		return nil, fmt.Errorf("parse REPLY_STREAM_SWEEP_ENABLED: %w", err)
	}

	return &Config{
		RedisURL:           getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		ConnectPort:        port,
		LogLevel:           getEnvOrDefault("LOG_LEVEL", "info"),
		RedisPoolSize:      poolSize,
		MaxBatchSize:       maxBatchSize,
		StreamMaxLen:       streamMaxLen,
		StreamHardMaxLen:   streamHardMaxLen,
		StreamTrimInterval:      time.Duration(trimIntervalSeconds) * time.Second,
		ReplyStreamSweepEnabled: replyStreamSweepEnabled,
	}, nil
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
