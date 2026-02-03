// Package config provides configuration management for mq-hub.
package config

import (
	"os"
	"strconv"
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
}

// NewConfig creates a new Config from environment variables.
func NewConfig() *Config {
	port, _ := strconv.Atoi(getEnvOrDefault("CONNECT_PORT", "9500"))
	poolSize, _ := strconv.Atoi(getEnvOrDefault("REDIS_POOL_SIZE", "10"))
	maxBatchSize, _ := strconv.Atoi(getEnvOrDefault("MAX_BATCH_SIZE", "1000"))

	return &Config{
		RedisURL:      getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
		ConnectPort:   port,
		LogLevel:      getEnvOrDefault("LOG_LEVEL", "info"),
		RedisPoolSize: poolSize,
		MaxBatchSize:  maxBatchSize,
	}
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
