// Package gateway provides anti-corruption layer implementations.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"mq-hub/domain"
	"mq-hub/driver"
	"mq-hub/port"
)

// ErrNilEvent is returned when attempting to publish a nil event. It wraps
// domain.ErrInvalidEvent so callers can classify it the same way as other
// event validation failures via errors.Is.
var ErrNilEvent = fmt.Errorf("nil event: %w", domain.ErrInvalidEvent)

// RedisStreamDriver defines the driver interface for Redis Streams I/O.
type RedisStreamDriver interface {
	Publish(ctx context.Context, stream string, msg *driver.StreamMessage) (string, error)
	PublishBatch(ctx context.Context, stream string, msgs []*driver.StreamMessage) ([]string, error)
	CreateConsumerGroup(ctx context.Context, stream string, group string, startID string) error
	GetStreamInfo(ctx context.Context, stream string) (*driver.StreamInfo, error)
	Ping(ctx context.Context) error
	SubscribeWithTimeout(ctx context.Context, stream string, timeout time.Duration) (*driver.StreamMessage, error)
	DeleteStream(ctx context.Context, stream string) error
	Expire(ctx context.Context, stream string, ttl time.Duration) error
	ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]string, error)
	TrimMaxLenApprox(ctx context.Context, stream string, maxLen int64) (int64, error)
}

// StreamGateway implements StreamPort, StreamTrimmer, and ReplyStreamSweeper using a driver.
type StreamGateway struct {
	driver RedisStreamDriver
}

// Compile-time checks for interface satisfaction.
var (
	_ port.StreamPort         = (*StreamGateway)(nil)
	_ port.StreamTrimmer      = (*StreamGateway)(nil)
	_ port.ReplyStreamSweeper = (*StreamGateway)(nil)
)

// NewStreamGateway creates a new StreamGateway.
func NewStreamGateway(driver RedisStreamDriver) *StreamGateway {
	return &StreamGateway{driver: driver}
}

// Publish publishes an event to a stream.
func (g *StreamGateway) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error) {
	// Validate stream key - log warning for unknown keys but allow for flexibility
	if !stream.IsValid() {
		slog.WarnContext(ctx, "publishing to unknown stream key",
			"stream", stream.String(),
		)
	}

	// Validate event
	if event == nil {
		return "", ErrNilEvent
	}
	if err := event.Validate(); err != nil {
		return "", err
	}

	msg, err := eventToDriverMessage(event)
	if err != nil {
		return "", err
	}

	return g.driver.Publish(ctx, stream.String(), msg)
}

// PublishBatch publishes multiple events to a stream.
// The returned slice always has one entry per input event so callers can
// correlate messageIDs[i] with events[i]; entries for events that failed to
// publish are left as "". If any event failed, the returned error is a
// *domain.PartialPublishError identifying exactly which indices failed, so
// callers can retry only those instead of re-publishing the whole batch
// (which would duplicate the events that already succeeded).
func (g *StreamGateway) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error) {
	// Validate stream key - log warning for unknown keys but allow for flexibility
	if !stream.IsValid() {
		slog.WarnContext(ctx, "publishing batch to unknown stream key",
			"stream", stream.String(),
			"batch_size", len(events),
		)
	}

	// Validate all events before publishing
	msgs := make([]*driver.StreamMessage, len(events))
	for i, event := range events {
		if event == nil {
			return nil, ErrNilEvent
		}
		if err := event.Validate(); err != nil {
			return nil, err
		}
		msg, err := eventToDriverMessage(event)
		if err != nil {
			return nil, fmt.Errorf("publish batch to %s: event at index %d: %w", stream, i, err)
		}
		msgs[i] = msg
	}

	ids, err := g.driver.PublishBatch(ctx, stream.String(), msgs)
	if err != nil {
		var partialErr *driver.PartialPublishError
		if errors.As(err, &partialErr) {
			failures := make([]domain.PublishFailure, len(partialErr.Failures))
			for i, f := range partialErr.Failures {
				failures[i] = domain.PublishFailure{Index: f.Index, Err: f.Err}
			}
			return ids, &domain.PartialPublishError{
				TotalEvents: partialErr.TotalMessages,
				Failures:    failures,
			}
		}
		return ids, err
	}

	return ids, nil
}

// CreateConsumerGroup creates a consumer group for a stream.
func (g *StreamGateway) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	return g.driver.CreateConsumerGroup(ctx, stream.String(), group.String(), startID)
}

// GetStreamInfo returns information about a stream.
func (g *StreamGateway) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	info, err := g.driver.GetStreamInfo(ctx, stream.String())
	if err != nil {
		return nil, err
	}

	groupInfos := make([]domain.ConsumerGroupInfo, 0, len(info.Groups))
	for _, grp := range info.Groups {
		groupInfos = append(groupInfos, domain.ConsumerGroupInfo{
			Name:            grp.Name,
			Consumers:       grp.Consumers,
			Pending:         grp.Pending,
			LastDeliveredID: grp.LastDeliveredID,
		})
	}

	return &domain.StreamInfo{
		Length:         info.Length,
		RadixTreeKeys:  info.RadixTreeKeys,
		RadixTreeNodes: info.RadixTreeNodes,
		FirstEntryID:   info.FirstEntryID,
		LastEntryID:    info.LastEntryID,
		Groups:         groupInfos,
	}, nil
}

// Ping checks if Redis is available.
func (g *StreamGateway) Ping(ctx context.Context) error {
	return g.driver.Ping(ctx)
}

// SubscribeWithTimeout waits for a message on a reply stream with timeout.
func (g *StreamGateway) SubscribeWithTimeout(ctx context.Context, stream domain.StreamKey, timeout time.Duration) (*domain.Event, error) {
	msg, err := g.driver.SubscribeWithTimeout(ctx, stream.String(), timeout)
	if err != nil {
		if errors.Is(err, driver.ErrReplyTimeout) {
			return nil, domain.ErrReplyTimeout
		}
		return nil, err
	}

	return driverMessageToEvent(msg), nil
}

// DeleteStream removes a stream.
func (g *StreamGateway) DeleteStream(ctx context.Context, stream domain.StreamKey) error {
	return g.driver.DeleteStream(ctx, stream.String())
}

// Expire sets a TTL on a stream key.
func (g *StreamGateway) Expire(ctx context.Context, stream domain.StreamKey, ttl time.Duration) error {
	return g.driver.Expire(ctx, stream.String(), ttl)
}

// TrimMaxLenApprox trims stream to approximately maxLen entries.
func (g *StreamGateway) TrimMaxLenApprox(ctx context.Context, stream domain.StreamKey, maxLen int64) (int64, error) {
	return g.driver.TrimMaxLenApprox(ctx, stream.String(), maxLen)
}

// ScanReplyStreamsWithoutTTL returns reply-stream keys that currently have no expiry set.
//
// It exists for the reply-stream safety-net sweep: a worker's late reply can
// XADD-recreate a request-reply stream after GenerateTagsForArticle's cleanup
// already deleted it, leaving a TTL-less key that the length-cap trim pass never
// touches (that pass only covers the fixed AllStreamKeys()). This scan finds
// exactly those keys so the sweep can re-apply a bounded TTL.
func (g *StreamGateway) ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]domain.StreamKey, error) {
	keys, err := g.driver.ScanReplyStreamsWithoutTTL(ctx, prefix)
	if err != nil {
		return nil, err
	}

	streamKeys := make([]domain.StreamKey, len(keys))
	for i, k := range keys {
		streamKeys[i] = domain.StreamKey(k)
	}
	return streamKeys, nil
}

// eventToDriverMessage converts a domain Event to a driver StreamMessage.
func eventToDriverMessage(event *domain.Event) (*driver.StreamMessage, error) {
	msg := &driver.StreamMessage{
		EventID:   event.EventID,
		EventType: string(event.EventType),
		Source:    event.Source,
		CreatedAt: event.CreatedAt.Format("2006-01-02T15:04:05.000Z07:00"),
	}

	if len(event.Payload) > 0 {
		msg.Payload = string(event.Payload)
	}

	if len(event.Metadata) > 0 {
		metadataJSON, err := json.Marshal(event.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal event metadata: %w", err)
		}
		msg.Metadata = string(metadataJSON)
	}

	return msg, nil
}

// driverMessageToEvent converts a driver StreamMessage to a domain Event.
func driverMessageToEvent(msg *driver.StreamMessage) *domain.Event {
	event := &domain.Event{
		EventID:  msg.EventID,
		Source:   msg.Source,
		Metadata: make(map[string]string),
	}

	if msg.EventType != "" {
		event.EventType = domain.EventType(msg.EventType)
	}

	if msg.CreatedAt != "" {
		if t, err := time.Parse("2006-01-02T15:04:05.000Z07:00", msg.CreatedAt); err == nil {
			event.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339, msg.CreatedAt); err == nil {
			event.CreatedAt = t
		}
	}

	if msg.Payload != "" {
		event.Payload = []byte(msg.Payload)
	}

	if msg.Metadata != "" {
		_ = json.Unmarshal([]byte(msg.Metadata), &event.Metadata)
	}

	return event
}
