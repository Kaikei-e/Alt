// Package usecase contains business logic for mq-hub.
package usecase

import (
	"context"
	"time"

	"mq-hub/domain"
	"mq-hub/port"
)

// PublishResult contains the result of publishing an event.
type PublishResult struct {
	MessageID string
	Success   bool
}

// PublishBatchResult contains the results of batch publishing.
type PublishBatchResult struct {
	MessageIDs   []string
	SuccessCount int32
	FailureCount int32
	Errors       []PublishError
}

// PublishError represents an error for a specific event in a batch.
type PublishError struct {
	Index        int
	ErrorMessage string
}

// HealthStatus contains the health status of the service.
type HealthStatus struct {
	Healthy       bool
	RedisStatus   string
	UptimeSeconds int64
}

// PublishUsecase handles event publishing operations.
type PublishUsecase struct {
	streamPort port.StreamPort
	startTime  time.Time
}

// NewPublishUsecase creates a new PublishUsecase.
func NewPublishUsecase(streamPort port.StreamPort) *PublishUsecase {
	return &PublishUsecase{
		streamPort: streamPort,
		startTime:  time.Now(),
	}
}

// Publish publishes a single event to a stream.
func (u *PublishUsecase) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (*PublishResult, error) {
	messageID, err := u.streamPort.Publish(ctx, stream, event)
	if err != nil {
		return &PublishResult{
			MessageID: "",
			Success:   false,
		}, err
	}

	return &PublishResult{
		MessageID: messageID,
		Success:   true,
	}, nil
}

// PublishBatch publishes multiple events to a stream.
func (u *PublishUsecase) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) (*PublishBatchResult, error) {
	messageIDs, err := u.streamPort.PublishBatch(ctx, stream, events)
	if err != nil {
		return &PublishBatchResult{
			MessageIDs:   nil,
			SuccessCount: 0,
			FailureCount: int32(len(events)),
		}, err
	}

	return &PublishBatchResult{
		MessageIDs:   messageIDs,
		SuccessCount: int32(len(messageIDs)),
		FailureCount: 0,
	}, nil
}

// CreateConsumerGroup creates a consumer group for a stream.
func (u *PublishUsecase) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	return u.streamPort.CreateConsumerGroup(ctx, stream, group, startID)
}

// GetStreamInfo returns information about a stream.
func (u *PublishUsecase) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	return u.streamPort.GetStreamInfo(ctx, stream)
}

// HealthCheck checks the health of the service.
func (u *PublishUsecase) HealthCheck(ctx context.Context) *HealthStatus {
	err := u.streamPort.Ping(ctx)

	status := &HealthStatus{
		UptimeSeconds: int64(time.Since(u.startTime).Seconds()),
	}

	if err != nil {
		status.Healthy = false
		status.RedisStatus = err.Error()
	} else {
		status.Healthy = true
		status.RedisStatus = "connected"
	}

	return status
}
