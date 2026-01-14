// Package consumer provides Redis Streams consumer for search-indexer.
package consumer

import (
	"os"
	"strconv"
	"time"
)

// Config holds consumer configuration.
type Config struct {
	// RedisURL is the Redis connection URL.
	RedisURL string
	// GroupName is the consumer group name.
	GroupName string
	// ConsumerName is this consumer's name within the group.
	ConsumerName string
	// StreamKey is the Redis Stream key to consume from.
	StreamKey string
	// BatchSize is the number of messages to read at once.
	BatchSize int64
	// BlockTimeout is how long to block waiting for messages.
	BlockTimeout time.Duration
	// ClaimIdleTime is the idle time for claiming pending messages (Redis 8.4 CLAIM option).
	ClaimIdleTime time.Duration
	// Enabled determines if the consumer is active.
	Enabled bool
}

// DefaultConfig returns a default consumer configuration.
func DefaultConfig() Config {
	return Config{
		RedisURL:      "redis://localhost:6379",
		GroupName:     "search-indexer-group",
		ConsumerName:  "search-indexer-1",
		StreamKey:     "alt:events:articles",
		BatchSize:     10,
		BlockTimeout:  5 * time.Second,
		ClaimIdleTime: 30 * time.Second,
		Enabled:       false,
	}
}

// ConfigFromEnv loads consumer configuration from environment variables.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("REDIS_STREAMS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("CONSUMER_GROUP"); v != "" {
		cfg.GroupName = v
	}
	if v := os.Getenv("CONSUMER_NAME"); v != "" {
		cfg.ConsumerName = v
	}
	if v := os.Getenv("CONSUMER_STREAM_KEY"); v != "" {
		cfg.StreamKey = v
	}
	if v := os.Getenv("CONSUMER_BATCH_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.BatchSize = n
		}
	}
	if v := os.Getenv("CONSUMER_ENABLED"); v != "" {
		cfg.Enabled = v == "true" || v == "1"
	}

	return cfg
}
