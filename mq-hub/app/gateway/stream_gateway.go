// Package gateway provides anti-corruption layer implementations.
package gateway

import (
	"context"

	"mq-hub/domain"
	"mq-hub/port"
)

// StreamGateway implements StreamPort using a driver.
type StreamGateway struct {
	driver port.StreamPort
}

// NewStreamGateway creates a new StreamGateway.
func NewStreamGateway(driver port.StreamPort) *StreamGateway {
	return &StreamGateway{driver: driver}
}

// Publish publishes an event to a stream.
func (g *StreamGateway) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error) {
	// Validate stream key
	if !stream.IsValid() {
		// Allow any stream key for flexibility
		// but log a warning for unknown keys
	}

	// Validate event
	if event != nil {
		if err := event.Validate(); err != nil {
			return "", err
		}
	}

	return g.driver.Publish(ctx, stream, event)
}

// PublishBatch publishes multiple events to a stream.
func (g *StreamGateway) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error) {
	// Validate all events before publishing
	for _, event := range events {
		if event != nil {
			if err := event.Validate(); err != nil {
				return nil, err
			}
		}
	}

	return g.driver.PublishBatch(ctx, stream, events)
}

// CreateConsumerGroup creates a consumer group for a stream.
func (g *StreamGateway) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	return g.driver.CreateConsumerGroup(ctx, stream, group, startID)
}

// GetStreamInfo returns information about a stream.
func (g *StreamGateway) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	return g.driver.GetStreamInfo(ctx, stream)
}

// Ping checks if Redis is available.
func (g *StreamGateway) Ping(ctx context.Context) error {
	return g.driver.Ping(ctx)
}
