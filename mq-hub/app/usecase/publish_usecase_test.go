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
