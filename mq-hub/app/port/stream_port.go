// Package port defines interfaces for external dependencies.
package port

import (
	"context"
	"time"

	"mq-hub/domain"
)

// StreamTrimmer bounds a stream's length independently of publishing.
//
// Kept separate from StreamPort on purpose: trimming is maintenance, not part of
// the publish/consume contract every StreamPort implementation has to satisfy.
type StreamTrimmer interface {
	// TrimMaxLenApprox trims stream to approximately maxLen entries and
	// returns how many were removed.
	TrimMaxLenApprox(ctx context.Context, stream domain.StreamKey, maxLen int64) (int64, error)
}

// ReplyStreamSweeper bounds the lifetime of temporary request-reply streams
// (usecase.ReplyStreamPrefix + correlationID).
//
// GenerateTagsForArticle deletes its reply stream on completion/timeout, but a
// worker replying late can XADD-recreate the key afterwards with no expiry, and
// the length-cap trim pass (StreamTrimmer) only covers the fixed set of streams
// in domain.AllStreamKeys(). Kept separate from StreamPort for the same reason
// as StreamTrimmer: this is maintenance, not part of the publish/consume
// contract every StreamPort implementation must satisfy.
type ReplyStreamSweeper interface {
	// ScanReplyStreamsWithoutTTL returns reply-stream keys matching prefix that
	// currently have no expiry set.
	ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]domain.StreamKey, error)

	// Expire sets a TTL on a stream key.
	Expire(ctx context.Context, stream domain.StreamKey, ttl time.Duration) error
}

// StreamPort defines the interface for Redis Streams operations.
type StreamPort interface {
	// Publish publishes an event to a stream and returns the message ID.
	Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error)

	// PublishBatch publishes multiple events to a stream and returns message IDs.
	PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error)

	// CreateConsumerGroup creates a consumer group for a stream.
	// startID can be "0" for beginning or "$" for new messages only.
	CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error

	// GetStreamInfo returns information about a stream.
	GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error)

	// Ping checks if Redis is available.
	Ping(ctx context.Context) error

	// SubscribeWithTimeout waits for a message on a reply stream with timeout.
	// Returns the first message received or an error if timeout expires.
	SubscribeWithTimeout(ctx context.Context, stream domain.StreamKey, timeout time.Duration) (*domain.Event, error)

	// DeleteStream removes a stream (used for cleanup of temporary reply streams).
	DeleteStream(ctx context.Context, stream domain.StreamKey) error

	// Expire sets a TTL on a stream key. Used as a safety net so temporary
	// reply streams are bounded even if DeleteStream cleanup fails or a
	// worker's late reply recreates the stream after cleanup already ran.
	Expire(ctx context.Context, stream domain.StreamKey, ttl time.Duration) error
}
