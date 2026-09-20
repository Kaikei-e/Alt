// Package config provides configuration management for mq-hub.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the configuration for mq-hub.
type Config struct {
	// RedisURL is the Redis connection URL.
	RedisURL string
	// RedisPassword is the authentication password for Redis, loaded from REDIS_PASSWORD_FILE.
	RedisPassword string
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

	redisPassword, err := ResolveRedisPassword(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve redis password: %w", err)
	}

	return &Config{
		RedisURL:                getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		RedisPassword:           redisPassword,
		ConnectPort:             port,
		LogLevel:                getEnvOrDefault("LOG_LEVEL", "info"),
		RedisPoolSize:           poolSize,
		MaxBatchSize:            maxBatchSize,
		StreamMaxLen:            streamMaxLen,
		StreamHardMaxLen:        streamHardMaxLen,
		StreamTrimInterval:      time.Duration(trimIntervalSeconds) * time.Second,
		ReplyStreamSweepEnabled: replyStreamSweepEnabled,
	}, nil
}

// ResolveRedisPassword resolves the Redis authentication password from the file
// referenced by REDIS_PASSWORD_FILE.
// If REDIS_AUTH=disabled is set (case-insensitive), it logs redis_auth_disabled once and returns an empty string.
// If REDIS_PASSWORD_FILE is set, it fails fast on missing, unreadable, or empty file contents (rules 8 and 9).
// If neither REDIS_PASSWORD_FILE nor REDIS_AUTH=disabled is set, it returns a startup error naming both variables.
func ResolveRedisPassword(logger *slog.Logger) (string, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if strings.EqualFold(strings.TrimSpace(os.Getenv("REDIS_AUTH")), "disabled") {
		logger.Warn("redis_auth_disabled", "reason", "REDIS_AUTH=disabled was set explicitly")
		return "", nil
	}

	filePath, set := os.LookupEnv("REDIS_PASSWORD_FILE")
	if !set {
		return "", fmt.Errorf("redis authentication requires REDIS_PASSWORD_FILE or REDIS_AUTH=disabled")
	}

	trimmedPath := strings.TrimSpace(filePath)
	if trimmedPath == "" {
		logger.Error("redis_password_file_empty", "env", "REDIS_PASSWORD_FILE")
		return "", fmt.Errorf("REDIS_PASSWORD_FILE is set but empty; set REDIS_AUTH=disabled to run with redis auth disabled explicitly")
	}

	content, err := os.ReadFile(trimmedPath)
	if err != nil {
		logger.Error("redis_password_file_read_failed", "file", trimmedPath, "error", err)
		return "", fmt.Errorf("read REDIS_PASSWORD_FILE %s: %w", trimmedPath, err)
	}

	password := strings.TrimSpace(string(content))
	if password == "" {
		logger.Error("redis_password_file_content_empty", "file", trimmedPath)
		return "", fmt.Errorf("REDIS_PASSWORD_FILE %s resolved to an empty password", trimmedPath)
	}

	return password, nil
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
