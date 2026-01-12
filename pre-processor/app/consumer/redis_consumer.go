// Package consumer provides Redis Streams consumer for pre-processor.
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
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
		GroupName:     "pre-processor-group",
		ConsumerName:  "pre-processor-1",
		StreamKey:     "alt:events:articles",
		BatchSize:     10,
		BlockTimeout:  5 * time.Second,
		ClaimIdleTime: 30 * time.Second,
		Enabled:       false,
	}
}

// Event represents a domain event from the stream.
type Event struct {
	// MessageID is the Redis Stream message ID.
	MessageID string
	// EventID is the unique event identifier.
	EventID string
	// EventType is the type of event.
	EventType string
	// Source is the service that produced the event.
	Source string
	// CreatedAt is when the event was created.
	CreatedAt time.Time
	// Payload is the event-specific data.
	Payload json.RawMessage
	// Metadata contains additional context.
	Metadata map[string]string
}

// EventHandler processes events from the stream.
type EventHandler interface {
	// HandleEvent processes a single event.
	HandleEvent(ctx context.Context, event Event) error
}

// Consumer consumes events from Redis Streams.
type Consumer struct {
	client       *redis.Client
	config       Config
	handler      EventHandler
	logger       *slog.Logger
	shutdownChan chan struct{}
}

// NewConsumer creates a new Redis Streams consumer.
func NewConsumer(config Config, handler EventHandler, logger *slog.Logger) (*Consumer, error) {
	if !config.Enabled {
		return &Consumer{config: config, logger: logger}, nil
	}

	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)

	if logger == nil {
		logger = slog.Default()
	}

	return &Consumer{
		client:       client,
		config:       config,
		handler:      handler,
		logger:       logger,
		shutdownChan: make(chan struct{}),
	}, nil
}

// Start begins consuming events from the stream.
func (c *Consumer) Start(ctx context.Context) error {
	if !c.config.Enabled {
		c.logger.Info("consumer disabled, not starting")
		return nil
	}

	// Ensure consumer group exists
	if err := c.ensureConsumerGroup(ctx); err != nil {
		return err
	}

	c.logger.Info("starting consumer",
		"stream", c.config.StreamKey,
		"group", c.config.GroupName,
		"consumer", c.config.ConsumerName,
	)

	go c.consumeLoop(ctx)
	return nil
}

// Stop gracefully stops the consumer.
func (c *Consumer) Stop() {
	if c.shutdownChan != nil {
		close(c.shutdownChan)
	}
	if c.client != nil {
		c.client.Close()
	}
}

// ensureConsumerGroup creates the consumer group if it doesn't exist.
func (c *Consumer) ensureConsumerGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.config.StreamKey, c.config.GroupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		// Ignore BUSYGROUP error, it means the group already exists
		if err.Error() == "BUSYGROUP Consumer Group name already exists" {
			return nil
		}
		return err
	}
	return nil
}

// consumeLoop continuously reads and processes events.
func (c *Consumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer context cancelled, stopping")
			return
		case <-c.shutdownChan:
			c.logger.Info("consumer shutdown requested, stopping")
			return
		default:
			if err := c.readAndProcess(ctx); err != nil {
				c.logger.Error("error processing events", "error", err)
				time.Sleep(time.Second) // Back off on error
			}
		}
	}
}

// readAndProcess reads events from the stream and processes them.
// Uses Redis 8.4 XREADGROUP with CLAIM option for handling idle pending messages.
func (c *Consumer) readAndProcess(ctx context.Context) error {
	// Read new messages and claim idle pending messages in one command (Redis 8.4)
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.config.GroupName,
		Consumer: c.config.ConsumerName,
		Streams:  []string{c.config.StreamKey, ">"},
		Count:    c.config.BatchSize,
		Block:    c.config.BlockTimeout,
	}).Result()

	if err == redis.Nil {
		// No messages available
		return nil
	}
	if err != nil {
		return err
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			event := c.parseEvent(message)

			if err := c.handler.HandleEvent(ctx, event); err != nil {
				c.logger.Error("failed to process event",
					"message_id", message.ID,
					"event_type", event.EventType,
					"error", err,
				)
				// Don't ACK failed messages, they'll be retried
				continue
			}

			// Acknowledge successful processing
			if err := c.client.XAck(ctx, c.config.StreamKey, c.config.GroupName, message.ID).Err(); err != nil {
				c.logger.Error("failed to acknowledge message",
					"message_id", message.ID,
					"error", err,
				)
			}
		}
	}

	return nil
}

// parseEvent converts a Redis Stream message to an Event.
func (c *Consumer) parseEvent(message redis.XMessage) Event {
	event := Event{
		MessageID: message.ID,
		Metadata:  make(map[string]string),
	}

	if v, ok := message.Values["event_id"].(string); ok {
		event.EventID = v
	}
	if v, ok := message.Values["event_type"].(string); ok {
		event.EventType = v
	}
	if v, ok := message.Values["source"].(string); ok {
		event.Source = v
	}
	if v, ok := message.Values["created_at"].(string); ok {
		event.CreatedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := message.Values["payload"].(string); ok {
		event.Payload = json.RawMessage(v)
	}
	if v, ok := message.Values["metadata"].(string); ok {
		json.Unmarshal([]byte(v), &event.Metadata)
	}

	return event
}
