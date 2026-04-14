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
	// DLQStreamKey is the Redis Stream key where poison messages are routed
	// after exceeding MaxDeliveries. Should be separate from StreamKey so the
	// main consumer does not keep reprocessing dead letters.
	DLQStreamKey string
	// MaxDeliveries is the maximum number of times a single message may be
	// delivered before it is moved to the DLQ. Zero disables DLQ routing.
	MaxDeliveries int64
	// ReaperInterval is how often the consumer scans the pending entries list
	// for messages that have exceeded MaxDeliveries.
	ReaperInterval time.Duration
}

// DefaultConfig returns a default consumer configuration.
func DefaultConfig() Config {
	return Config{
		RedisURL:       "redis://localhost:6379",
		GroupName:      "search-indexer-group",
		ConsumerName:   "search-indexer-1",
		StreamKey:      "alt:events:articles",
		BatchSize:      10,
		BlockTimeout:   5 * time.Second,
		ClaimIdleTime:  30 * time.Second,
		Enabled:        false,
		DLQStreamKey:   "alt:events:articles:dlq",
		MaxDeliveries:  5,
		ReaperInterval: 60 * time.Second,
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
	if v := os.Getenv("CONSUMER_DLQ_STREAM"); v != "" {
		cfg.DLQStreamKey = v
	}
	if v := os.Getenv("CONSUMER_MAX_DELIVERIES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxDeliveries = n
		}
	}
	if v := os.Getenv("CONSUMER_REAPER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ReaperInterval = d
		}
	}

	return cfg
}
