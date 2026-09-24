// Package driver provides implementations for external dependencies.
package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamMessage represents a raw message entry stored in or read from Redis Streams.
type StreamMessage struct {
	ID        string
	EventID   string
	EventType string
	Source    string
	CreatedAt string
	Payload   string
	Metadata  string
}

// StreamInfo holds information about a Redis stream.
type StreamInfo struct {
	Length         int64
	RadixTreeKeys  int64
	RadixTreeNodes int64
	FirstEntryID   string
	LastEntryID    string
	Groups         []ConsumerGroupInfo
}

// ConsumerGroupInfo holds information about a consumer group.
type ConsumerGroupInfo struct {
	Name            string
	Consumers       int64
	Pending         int64
	LastDeliveredID string
}

// PublishFailure records a failed publication in a batch.
type PublishFailure struct {
	Index int
	Err   error
}

// PartialPublishError is returned when one or more messages fail to publish in a batch.
type PartialPublishError struct {
	TotalMessages int
	Failures      []PublishFailure
}

func (e *PartialPublishError) Error() string {
	return fmt.Sprintf("partial publish: %d of %d messages failed", len(e.Failures), e.TotalMessages)
}

// ErrReplyTimeout is returned when waiting for a reply message times out.
var ErrReplyTimeout = errors.New("timeout waiting for reply message")

// RedisDriver handles Redis Streams I/O.
type RedisDriver struct {
	client       *redis.Client
	streamMaxLen int64
}

// RedisDriverOptions contains configuration for Redis driver.
type RedisDriverOptions struct {
	PoolSize     int
	StreamMaxLen int64
	Password     string
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

	if opts != nil {
		if opts.PoolSize > 0 {
			redisOpts.PoolSize = opts.PoolSize
		}
		if opts.Password != "" {
			redisOpts.Password = opts.Password
		}
	}

	client := redis.NewClient(redisOpts)

	d := &RedisDriver{client: client}
	if opts != nil && opts.StreamMaxLen > 0 {
		d.streamMaxLen = opts.StreamMaxLen
	}

	return d, nil
}

// NewRedisDriverWithURL creates a new Redis driver from a URL.
func NewRedisDriverWithURL(url string) (*RedisDriver, error) {
	return NewRedisDriverWithURLAndOptions(url, nil)
}

// NewRedisDriverWithURLAndOptions creates a new Redis driver from a URL with options.
func NewRedisDriverWithURLAndOptions(url string, driverOpts *RedisDriverOptions) (*RedisDriver, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	if driverOpts != nil {
		if driverOpts.PoolSize > 0 {
			opts.PoolSize = driverOpts.PoolSize
		}
		if driverOpts.Password != "" {
			opts.Password = driverOpts.Password
		}
	}

	client := redis.NewClient(opts)

	d := &RedisDriver{client: client}
	if driverOpts != nil && driverOpts.StreamMaxLen > 0 {
		d.streamMaxLen = driverOpts.StreamMaxLen
	}

	return d, nil
}

// Close closes the Redis connection.
func (d *RedisDriver) Close() error {
	return d.client.Close()
}

// Publish publishes a message to a stream and returns the message ID.
func (d *RedisDriver) Publish(ctx context.Context, stream string, msg *StreamMessage) (string, error) {
	if msg == nil {
		return "", errors.New("message is nil")
	}

	values := d.messageToValues(msg)

	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}
	if d.streamMaxLen > 0 {
		args.MaxLen = d.streamMaxLen
		args.Approx = true
		// ACKED restricts approximate MAXLEN trimming to entries every
		// consumer group has read and acked (Redis 8.2+). Without it, a
		// stalled/backlogged consumer's undelivered or unacked entries are
		// silently evicted once the stream exceeds streamMaxLen.
		args.Mode = "ACKED"
	}

	result, err := d.client.XAdd(ctx, args).Result()
	if err != nil {
		return "", fmt.Errorf("xadd %s: %w", stream, err)
	}

	return result, nil
}

// PublishBatch publishes multiple messages to a stream and returns message IDs.
// The returned slice always has one entry per input message so callers can
// correlate messageIDs[i] with msgs[i]; entries for messages that failed to
// publish are left as "". If any message failed, the returned error is a
// *PartialPublishError identifying exactly which indices failed, so
// callers can retry only those instead of re-publishing the whole batch
// (which would duplicate the messages that already succeeded).
func (d *RedisDriver) PublishBatch(ctx context.Context, stream string, msgs []*StreamMessage) ([]string, error) {
	if len(msgs) == 0 {
		return []string{}, nil
	}

	messageIDs := make([]string, len(msgs))
	cmds := make([]*redis.StringCmd, len(msgs))

	// Use pipeline for efficient batch publishing
	pipe := d.client.Pipeline()

	for i, msg := range msgs {
		if msg == nil {
			return nil, fmt.Errorf("publish batch to %s: message at index %d is nil", stream, i)
		}
		values := d.messageToValues(msg)
		args := &redis.XAddArgs{
			Stream: stream,
			Values: values,
		}
		if d.streamMaxLen > 0 {
			args.MaxLen = d.streamMaxLen
			args.Approx = true
			// See Publish: ACKED keeps undelivered/unacked backlog entries
			// out of reach of approximate MAXLEN trimming.
			args.Mode = "ACKED"
		}
		cmds[i] = pipe.XAdd(ctx, args)
	}

	// Exec only reports whether the pipeline as a whole had an error; it does
	// not tell us which individual XADD commands failed. Redis still runs
	// every queued command even when one fails, so we must inspect each
	// cmd.Err() to know which events actually landed.
	_, execErr := pipe.Exec(ctx)

	var failures []PublishFailure
	for i, cmd := range cmds {
		if err := cmd.Err(); err != nil {
			failures = append(failures, PublishFailure{Index: i, Err: err})
			continue
		}
		messageIDs[i] = cmd.Val()
	}

	if len(failures) > 0 {
		return messageIDs, &PartialPublishError{TotalMessages: len(msgs), Failures: failures}
	}
	if execErr != nil {
		return messageIDs, fmt.Errorf("publish batch to %s: %w", stream, execErr)
	}

	return messageIDs, nil
}

// CreateConsumerGroup creates a consumer group for a stream.
func (d *RedisDriver) CreateConsumerGroup(ctx context.Context, stream string, group string, startID string) error {
	err := d.client.XGroupCreateMkStream(ctx, stream, group, startID).Err()
	if err != nil {
		if isBusyGroupErr(err) {
			return nil
		}
		return fmt.Errorf("create consumer group %s on %s: %w", group, stream, err)
	}
	return nil
}

// GetStreamInfo returns information about a stream.
func (d *RedisDriver) GetStreamInfo(ctx context.Context, stream string) (*StreamInfo, error) {
	info, err := d.client.XInfoStream(ctx, stream).Result()
	if err != nil {
		return nil, fmt.Errorf("xinfo stream %s: %w", stream, err)
	}

	// Get consumer group info
	groups, err := d.client.XInfoGroups(ctx, stream).Result()
	if err != nil && !isNoSuchKeyErr(err) {
		return nil, fmt.Errorf("xinfo groups %s: %w", stream, err)
	}

	groupInfos := make([]ConsumerGroupInfo, 0, len(groups))
	for _, g := range groups {
		groupInfos = append(groupInfos, ConsumerGroupInfo{
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

	return &StreamInfo{
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

// messageToValues converts a StreamMessage to a map for XADD.
func (d *RedisDriver) messageToValues(msg *StreamMessage) map[string]interface{} {
	values := map[string]interface{}{
		"event_id":   msg.EventID,
		"event_type": msg.EventType,
		"source":     msg.Source,
		"created_at": msg.CreatedAt,
	}

	if msg.Payload != "" {
		values["payload"] = msg.Payload
	}

	if msg.Metadata != "" {
		values["metadata"] = msg.Metadata
	}

	return values
}

// isBusyGroupErr reports whether err is Redis BUSYGROUP (group already exists).
// go-redis exposes no typed sentinel for this reply, so callers must match the
// reply prefix. Isolate the comparison here rather than scattering .Error()
// checks through business code (DECREE §2).
func isBusyGroupErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "BUSYGROUP")
}

// isNoSuchKeyErr reports whether err is Redis "no such key".
// go-redis exposes no typed sentinel for this reply, so callers must match the
// reply text. Isolate the comparison here (DECREE §2).
func isNoSuchKeyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such key") || strings.HasPrefix(msg, "ERR no such key")
}

// SubscribeWithTimeout waits for a message on a reply stream with timeout.
// Uses XREAD with blocking to wait for messages.
func (d *RedisDriver) SubscribeWithTimeout(ctx context.Context, stream string, timeout time.Duration) (*StreamMessage, error) {
	streams, err := d.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{stream, "0"},
		Count:   1,
		Block:   timeout,
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrReplyTimeout
		}
		return nil, err
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, errors.New("no messages received")
	}

	msg := streams[0].Messages[0]
	return d.parseMessage(msg), nil
}

// DeleteStream removes a stream (used for cleanup of temporary reply streams).
func (d *RedisDriver) DeleteStream(ctx context.Context, stream string) error {
	return d.client.Del(ctx, stream).Err()
}

// Expire sets a TTL on a stream key. It is a no-op (no error) if the key
// does not exist yet.
func (d *RedisDriver) Expire(ctx context.Context, stream string, ttl time.Duration) error {
	return d.client.Expire(ctx, stream, ttl).Err()
}

// replyStreamScanCount bounds how many keys each SCAN cursor step asks Redis to
// examine. SCAN is cursor-based and non-blocking, so this only caps per-call
// work; the caller loops until the cursor returns to 0.
const replyStreamScanCount = 100

// ScanReplyStreamsWithoutTTL returns every key matching prefix+"*" that
// currently has no expiry set (TTL == -1). Missing keys (TTL == -2, a SCAN/TTL
// race) are skipped.
//
// It exists for the reply-stream safety-net sweep: a worker's late reply can
// XADD-recreate a request-reply stream after GenerateTagsForArticle's cleanup
// already deleted it, leaving a TTL-less key that the length-cap trim pass never
// touches (that pass only covers the fixed AllStreamKeys()). This scan finds
// exactly those keys so the sweep can re-apply a bounded TTL.
func (d *RedisDriver) ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]string, error) {
	match := prefix + "*"
	var (
		leaked []string
		cursor uint64
	)
	for {
		batch, next, err := d.client.Scan(ctx, cursor, match, replyStreamScanCount).Result()
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", match, err)
		}
		for _, key := range batch {
			ttl, err := d.client.TTL(ctx, key).Result()
			if err != nil {
				return nil, fmt.Errorf("ttl %s: %w", key, err)
			}
			// go-redis maps Redis's TTL reply of -1 (key exists, no expiry) to
			// -1ns and -2 (key missing) to -2ns. Only the no-expiry case needs
			// a safety-net TTL applied.
			if ttl == -1*time.Nanosecond {
				leaked = append(leaked, key)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return leaked, nil
}

// parseMessage converts a Redis stream message to a StreamMessage.
func (d *RedisDriver) parseMessage(msg redis.XMessage) *StreamMessage {
	return &StreamMessage{
		ID:        msg.ID,
		EventID:   getStringValue(msg.Values, "event_id"),
		EventType: getStringValue(msg.Values, "event_type"),
		Source:    getStringValue(msg.Values, "source"),
		CreatedAt: getStringValue(msg.Values, "created_at"),
		Payload:   getStringValue(msg.Values, "payload"),
		Metadata:  getStringValue(msg.Values, "metadata"),
	}
}

// TrimMaxLenApprox trims stream to approximately maxLen entries, ignoring
// consumer references, and returns how many entries were removed.
//
// XTRIM is not a denyoom command, so unlike the trim carried on XADD this keeps
// working while the instance is at maxmemory — which is the only state where a
// stream needs trimming and cannot get it from the publish path.
//
// LIMIT is left unset so Redis applies its default effort cap. The caller runs
// this on a timer, so converging over a few bounded passes is preferable to one
// unbounded pass blocking a single-threaded server.
func (d *RedisDriver) TrimMaxLenApprox(ctx context.Context, stream string, maxLen int64) (int64, error) {
	deleted, err := d.client.XTrimMaxLenApprox(ctx, stream, maxLen, 0).Result()
	if err != nil {
		return 0, fmt.Errorf("xtrim %s maxlen ~ %d: %w", stream, maxLen, err)
	}
	return deleted, nil
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
