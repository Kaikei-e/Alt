// Package driver provides implementations for external dependencies.
package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"mq-hub/domain"
)

// RedisDriver implements StreamPort using Redis Streams.
type RedisDriver struct {
	client *redis.Client
}

// RedisDriverOptions contains configuration for Redis driver.
type RedisDriverOptions struct {
	PoolSize int
}

// NewRedisDriver creates a new Redis driver.
func NewRedisDriver(addr string) (*RedisDriver, error) {
	return NewRedisDriverWithOptions(addr, nil)
}

// NewRedisDriverWithOptions creates a new Redis driver with options.
func NewRedisDriverWithOptions(addr string, opts *RedisDriverOptions) (*RedisDriver, error) {
	redisOpts := &redis.Options{
		Addr: addr,
	}

	if opts != nil && opts.PoolSize > 0 {
		redisOpts.PoolSize = opts.PoolSize
	}

	client := redis.NewClient(redisOpts)

	return &RedisDriver{client: client}, nil
}

// NewRedisDriverWithURL creates a new Redis driver from a URL.
func NewRedisDriverWithURL(url string) (*RedisDriver, error) {
	return NewRedisDriverWithURLAndOptions(url, nil)
}

// NewRedisDriverWithURLAndOptions creates a new Redis driver from a URL with options.
func NewRedisDriverWithURLAndOptions(url string, driverOpts *RedisDriverOptions) (*RedisDriver, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	if driverOpts != nil && driverOpts.PoolSize > 0 {
		opts.PoolSize = driverOpts.PoolSize
	}

	client := redis.NewClient(opts)

	return &RedisDriver{client: client}, nil
}

// Close closes the Redis connection.
func (d *RedisDriver) Close() error {
	return d.client.Close()
}

// Publish publishes an event to a stream and returns the message ID.
func (d *RedisDriver) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error) {
	if event == nil {
		return "", errors.New("event is nil")
	}

	values := d.eventToValues(event)

	result, err := d.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream.String(),
		Values: values,
	}).Result()
	if err != nil {
		return "", err
	}

	return result, nil
}

// PublishBatch publishes multiple events to a stream and returns message IDs.
func (d *RedisDriver) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error) {
	if len(events) == 0 {
		return []string{}, nil
	}

	messageIDs := make([]string, 0, len(events))

	// Use pipeline for efficient batch publishing
	pipe := d.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(events))

	for i, event := range events {
		if event == nil {
			continue
		}
		values := d.eventToValues(event)
		cmds[i] = pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: stream.String(),
			Values: values,
		})
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	for _, cmd := range cmds {
		if cmd != nil {
			messageIDs = append(messageIDs, cmd.Val())
		}
	}

	return messageIDs, nil
}

// CreateConsumerGroup creates a consumer group for a stream.
func (d *RedisDriver) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	err := d.client.XGroupCreateMkStream(ctx, stream.String(), group.String(), startID).Err()
	if err != nil {
		// Handle BUSYGROUP error (group already exists)
		if strings.Contains(err.Error(), "BUSYGROUP") {
			return nil
		}
		return err
	}
	return nil
}

// GetStreamInfo returns information about a stream.
func (d *RedisDriver) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	info, err := d.client.XInfoStream(ctx, stream.String()).Result()
	if err != nil {
		return nil, err
	}

	// Get consumer group info
	groups, err := d.client.XInfoGroups(ctx, stream.String()).Result()
	if err != nil && !strings.Contains(err.Error(), "no such key") {
		return nil, err
	}

	groupInfos := make([]domain.ConsumerGroupInfo, 0, len(groups))
	for _, g := range groups {
		groupInfos = append(groupInfos, domain.ConsumerGroupInfo{
			Name:            g.Name,
			Consumers:       g.Consumers,
			Pending:         g.Pending,
			LastDeliveredID: g.LastDeliveredID,
		})
	}

	firstEntryID := ""
	if info.FirstEntry.ID != "" {
		firstEntryID = info.FirstEntry.ID
	}

	lastEntryID := ""
	if info.LastEntry.ID != "" {
		lastEntryID = info.LastEntry.ID
	}

	return &domain.StreamInfo{
		Length:         info.Length,
		RadixTreeKeys:  info.RadixTreeKeys,
		RadixTreeNodes: info.RadixTreeNodes,
		FirstEntryID:   firstEntryID,
		LastEntryID:    lastEntryID,
		Groups:         groupInfos,
	}, nil
}

// Ping checks if Redis is available.
func (d *RedisDriver) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

// eventToValues converts an Event to a map for XADD.
func (d *RedisDriver) eventToValues(event *domain.Event) map[string]interface{} {
	values := map[string]interface{}{
		"event_id":   event.EventID,
		"event_type": string(event.EventType),
		"source":     event.Source,
		"created_at": event.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
	}

	if len(event.Payload) > 0 {
		values["payload"] = string(event.Payload)
	}

	if len(event.Metadata) > 0 {
		metadataJSON, _ := json.Marshal(event.Metadata)
		values["metadata"] = string(metadataJSON)
	}

	return values
}

// SubscribeWithTimeout waits for a message on a reply stream with timeout.
// Uses XREAD with blocking to wait for messages.
func (d *RedisDriver) SubscribeWithTimeout(ctx context.Context, stream domain.StreamKey, timeout time.Duration) (*domain.Event, error) {
	// Use XREAD with block timeout to wait for messages
	streams, err := d.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{stream.String(), "0"},
		Count:   1,
		Block:   timeout,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errors.New("timeout waiting for reply")
		}
		return nil, err
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, errors.New("no messages received")
	}

	msg := streams[0].Messages[0]
	return d.parseEventFromMessage(msg), nil
}

// DeleteStream removes a stream (used for cleanup of temporary reply streams).
func (d *RedisDriver) DeleteStream(ctx context.Context, stream domain.StreamKey) error {
	return d.client.Del(ctx, stream.String()).Err()
}

// parseEventFromMessage converts a Redis stream message to a domain Event.
func (d *RedisDriver) parseEventFromMessage(msg redis.XMessage) *domain.Event {
	event := &domain.Event{
		EventID:  getStringValue(msg.Values, "event_id"),
		Source:   getStringValue(msg.Values, "source"),
		Metadata: make(map[string]string),
	}

	if eventType := getStringValue(msg.Values, "event_type"); eventType != "" {
		event.EventType = domain.EventType(eventType)
	}

	if createdAtStr := getStringValue(msg.Values, "created_at"); createdAtStr != "" {
		if t, err := time.Parse("2006-01-02T15:04:05.000Z07:00", createdAtStr); err == nil {
			event.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			event.CreatedAt = t
		}
	}

	if payload := getStringValue(msg.Values, "payload"); payload != "" {
		event.Payload = []byte(payload)
	}

	if metadataStr := getStringValue(msg.Values, "metadata"); metadataStr != "" {
		_ = json.Unmarshal([]byte(metadataStr), &event.Metadata)
	}

	return event
}

// getStringValue safely extracts a string value from a map.
func getStringValue(values map[string]interface{}, key string) string {
	if v, ok := values[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
