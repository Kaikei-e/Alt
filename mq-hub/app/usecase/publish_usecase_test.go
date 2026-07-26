package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"mq-hub/domain"
)

// MockStreamPort is a mock implementation of port.StreamPort.
type MockStreamPort struct {
	mock.Mock
}

func (m *MockStreamPort) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error) {
	args := m.Called(ctx, stream, event)
	return args.String(0), args.Error(1)
}

func (m *MockStreamPort) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error) {
	args := m.Called(ctx, stream, events)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockStreamPort) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	args := m.Called(ctx, stream, group, startID)
	return args.Error(0)
}

func (m *MockStreamPort) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	args := m.Called(ctx, stream)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.StreamInfo), args.Error(1)
}

func (m *MockStreamPort) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStreamPort) SubscribeWithTimeout(ctx context.Context, stream domain.StreamKey, timeout time.Duration) (*domain.Event, error) {
	args := m.Called(ctx, stream, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}

func (m *MockStreamPort) DeleteStream(ctx context.Context, stream domain.StreamKey) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}

func (m *MockStreamPort) Expire(ctx context.Context, stream domain.StreamKey, ttl time.Duration) error {
	args := m.Called(ctx, stream, ttl)
	return args.Error(0)
}

func TestPublishUsecase_Publish(t *testing.T) {
	t.Run("publishes event successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "test-1",
			EventType: domain.EventTypeArticleCreated,
			Source:    "alt-backend",
			CreatedAt: time.Now(),
			Payload:   []byte(`{"article_id": "123"}`),
		}

		mockPort.On("Publish", ctx, domain.StreamKeyArticles, event).Return("1234567890123-0", nil)

		result, err := uc.Publish(ctx, domain.StreamKeyArticles, event)

		require.NoError(t, err)
		assert.Equal(t, "1234567890123-0", result.MessageID)
		assert.True(t, result.Success)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when publish fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()
		event := &domain.Event{
			EventID:   "test-1",
			EventType: domain.EventTypeArticleCreated,
			Source:    "alt-backend",
			CreatedAt: time.Now(),
		}

		mockPort.On("Publish", ctx, domain.StreamKeyArticles, event).Return("", errors.New("redis error"))

		result, err := uc.Publish(ctx, domain.StreamKeyArticles, event)

		require.Error(t, err)
		assert.Empty(t, result.MessageID)
		assert.False(t, result.Success)
		mockPort.AssertExpectations(t)
	})
}

func TestPublishUsecase_PublishBatch(t *testing.T) {
	t.Run("publishes batch successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()
		events := []*domain.Event{
			{
				EventID:   "test-1",
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
			{
				EventID:   "test-2",
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
		}

		mockPort.On("PublishBatch", ctx, domain.StreamKeyArticles, events).
			Return([]string{"123-0", "123-1"}, nil)

		result, err := uc.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.NoError(t, err)
		assert.Len(t, result.MessageIDs, 2)
		assert.Equal(t, int32(2), result.SuccessCount)
		assert.Equal(t, int32(0), result.FailureCount)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns partial result when some events in the batch fail", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()
		events := []*domain.Event{
			{
				EventID:   "test-1",
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
			{
				EventID:   "test-2",
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
			{
				EventID:   "test-3",
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
		}

		partialErr := &domain.PartialPublishError{
			TotalEvents: 3,
			Failures: []domain.PublishFailure{
				{Index: 1, Err: errors.New("connection reset")},
			},
		}
		// The driver still returns a messageIDs slice with one entry per
		// event; failed indices are left as "" so callers can tell which
		// events actually landed (see RedisDriver.PublishBatch doc).
		mockPort.On("PublishBatch", ctx, domain.StreamKeyArticles, events).
			Return([]string{"123-0", "", "123-2"}, partialErr)

		result, err := uc.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.Error(t, err)
		var gotPartialErr *domain.PartialPublishError
		require.ErrorAs(t, err, &gotPartialErr, "PublishBatch must surface the partial error typed so handlers can classify it")
		assert.Equal(t, []string{"123-0", "", "123-2"}, result.MessageIDs)
		assert.Equal(t, int32(2), result.SuccessCount, "2 of 3 events landed")
		assert.Equal(t, int32(1), result.FailureCount)
		require.Len(t, result.Errors, 1)
		assert.Equal(t, 1, result.Errors[0].Index)
		assert.Equal(t, "connection reset", result.Errors[0].ErrorMessage)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when batch size exceeds limit", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecaseWithOptions(mockPort, &PublishUsecaseOptions{
			MaxBatchSize: 2,
		})

		ctx := context.Background()
		events := make([]*domain.Event, 3) // 3 events, limit is 2
		for i := range events {
			events[i] = &domain.Event{
				EventID:   "test-" + string(rune('1'+i)),
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			}
		}

		result, err := uc.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.Error(t, err)
		assert.Equal(t, ErrBatchTooLarge, err)
		assert.Nil(t, result.MessageIDs)
		assert.Equal(t, int32(0), result.SuccessCount)
		assert.Equal(t, int32(3), result.FailureCount)
		// Should not call the underlying port
		mockPort.AssertNotCalled(t, "PublishBatch")
	})

	t.Run("respects custom max batch size", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecaseWithOptions(mockPort, &PublishUsecaseOptions{
			MaxBatchSize: 5,
		})

		ctx := context.Background()
		events := make([]*domain.Event, 5) // exactly at limit
		for i := range events {
			events[i] = &domain.Event{
				EventID:   "test-" + string(rune('1'+i)),
				EventType: domain.EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			}
		}

		mockPort.On("PublishBatch", ctx, domain.StreamKeyArticles, events).
			Return([]string{"1-0", "1-1", "1-2", "1-3", "1-4"}, nil)

		result, err := uc.PublishBatch(ctx, domain.StreamKeyArticles, events)

		require.NoError(t, err)
		assert.Len(t, result.MessageIDs, 5)
		mockPort.AssertExpectations(t)
	})
}

func TestPublishUsecase_CreateConsumerGroup(t *testing.T) {
	t.Run("creates consumer group successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()

		mockPort.On("CreateConsumerGroup", ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0").
			Return(nil)

		err := uc.CreateConsumerGroup(ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0")

		require.NoError(t, err)
		mockPort.AssertExpectations(t)
	})

	t.Run("propagates driver error", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()

		mockPort.On("CreateConsumerGroup", ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0").
			Return(errors.New("stream does not exist"))

		err := uc.CreateConsumerGroup(ctx, domain.StreamKeyArticles, domain.ConsumerGroupPreProcessor, "0")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "stream does not exist")
		mockPort.AssertExpectations(t)
	})
}

func TestPublishUsecase_GetStreamInfo(t *testing.T) {
	t.Run("returns stream info", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()
		expectedInfo := &domain.StreamInfo{
			Length:       100,
			FirstEntryID: "123-0",
			LastEntryID:  "123-99",
		}

		mockPort.On("GetStreamInfo", ctx, domain.StreamKeyArticles).Return(expectedInfo, nil)

		info, err := uc.GetStreamInfo(ctx, domain.StreamKeyArticles)

		require.NoError(t, err)
		assert.Equal(t, int64(100), info.Length)
		mockPort.AssertExpectations(t)
	})

	t.Run("propagates driver error", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()

		mockPort.On("GetStreamInfo", ctx, domain.StreamKeyArticles).
			Return(nil, errors.New("stream not found"))

		info, err := uc.GetStreamInfo(ctx, domain.StreamKeyArticles)

		require.Error(t, err)
		assert.Nil(t, info)
		mockPort.AssertExpectations(t)
	})
}

func TestPublishUsecase_HealthCheck(t *testing.T) {
	t.Run("returns healthy when Redis is available", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()

		mockPort.On("Ping", ctx).Return(nil)

		health := uc.HealthCheck(ctx)

		assert.True(t, health.Healthy)
		assert.Equal(t, "connected", health.RedisStatus)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns unhealthy when Redis is unavailable", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := NewPublishUsecase(mockPort)

		ctx := context.Background()

		mockPort.On("Ping", ctx).Return(errors.New("connection refused"))

		health := uc.HealthCheck(ctx)

		assert.False(t, health.Healthy)
		assert.Equal(t, "connection refused", health.RedisStatus)
		mockPort.AssertExpectations(t)
	})
}
