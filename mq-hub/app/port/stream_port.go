// Package port defines interfaces for external dependencies.
package port

import (
	"context"

	"mq-hub/domain"
)

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
}
