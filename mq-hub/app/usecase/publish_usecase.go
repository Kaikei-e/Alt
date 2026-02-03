// Package usecase contains business logic for mq-hub.
package usecase

import (
	"context"
	"errors"
	"time"

	"mq-hub/domain"
	"mq-hub/metrics"
	"mq-hub/port"
)

// ErrBatchTooLarge is returned when batch size exceeds the limit.
var ErrBatchTooLarge = errors.New("batch size exceeds maximum allowed")

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

// PublishUsecaseOptions contains configuration for PublishUsecase.
type PublishUsecaseOptions struct {
	MaxBatchSize int
}

// PublishUsecase handles event publishing operations.
type PublishUsecase struct {
	streamPort   port.StreamPort
	startTime    time.Time
	maxBatchSize int
}

// NewPublishUsecase creates a new PublishUsecase with default options.
func NewPublishUsecase(streamPort port.StreamPort) *PublishUsecase {
	return NewPublishUsecaseWithOptions(streamPort, nil)
}

// NewPublishUsecaseWithOptions creates a new PublishUsecase with options.
func NewPublishUsecaseWithOptions(streamPort port.StreamPort, opts *PublishUsecaseOptions) *PublishUsecase {
	maxBatchSize := 1000 // default
	if opts != nil && opts.MaxBatchSize > 0 {
		maxBatchSize = opts.MaxBatchSize
	}

	return &PublishUsecase{
		streamPort:   streamPort,
		startTime:    time.Now(),
		maxBatchSize: maxBatchSize,
	}
}

// Publish publishes a single event to a stream.
func (u *PublishUsecase) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (*PublishResult, error) {
	start := time.Now()

	messageID, err := u.streamPort.Publish(ctx, stream, event)
	duration := time.Since(start).Seconds()

	if err != nil {
		metrics.RecordPublish(stream.String(), "error", duration)
		metrics.RecordError("publish", "redis_error")
		return &PublishResult{
			MessageID: "",
			Success:   false,
		}, err
	}

	metrics.RecordPublish(stream.String(), "success", duration)
	return &PublishResult{
		MessageID: messageID,
		Success:   true,
	}, nil
}

// PublishBatch publishes multiple events to a stream.
func (u *PublishUsecase) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) (*PublishBatchResult, error) {
	batchSize := len(events)

	// Check batch size limit
	if batchSize > u.maxBatchSize {
		metrics.RecordError("publish_batch", "batch_too_large")
		return &PublishBatchResult{
			MessageIDs:   nil,
			SuccessCount: 0,
			FailureCount: int32(batchSize),
		}, ErrBatchTooLarge
	}

	start := time.Now()

	messageIDs, err := u.streamPort.PublishBatch(ctx, stream, events)
	duration := time.Since(start).Seconds()

	if err != nil {
		metrics.RecordBatchPublish(stream.String(), "error", batchSize, duration)
		metrics.RecordError("publish_batch", "redis_error")
		return &PublishBatchResult{
			MessageIDs:   nil,
			SuccessCount: 0,
			FailureCount: int32(batchSize),
		}, err
	}

	metrics.RecordBatchPublish(stream.String(), "success", batchSize, duration)
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
		metrics.SetRedisDisconnected()
	} else {
		status.Healthy = true
		status.RedisStatus = "connected"
		metrics.SetRedisConnected()
	}

	return status
}
