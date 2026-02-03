package mqhub

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"mq-hub/domain"
	mqhubv1 "mq-hub/gen/proto/services/mqhub/v1"
	"mq-hub/usecase"
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
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
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

func TestHandler_Publish(t *testing.T) {
	t.Run("publishes event successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()
		now := time.Now()

		mockPort.On("Publish", ctx, domain.StreamKey("articles"), mock.AnythingOfType("*domain.Event")).
			Return("1234567890123-0", nil)

		req := connect.NewRequest(&mqhubv1.PublishRequest{
			Stream: "articles",
			Event: &mqhubv1.Event{
				EventId:   "test-1",
				EventType: "ArticleCreated",
				Source:    "alt-backend",
				CreatedAt: timestamppb.New(now),
				Payload:   []byte(`{"article_id": "123"}`),
			},
		})

		resp, err := handler.Publish(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, "1234567890123-0", resp.Msg.MessageId)
		assert.True(t, resp.Msg.Success)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when event is nil", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		req := connect.NewRequest(&mqhubv1.PublishRequest{
			Stream: "articles",
			Event:  nil,
		})

		resp, err := handler.Publish(ctx, req)

		require.Error(t, err)
		assert.False(t, resp.Msg.Success)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("returns error when publish fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("Publish", ctx, domain.StreamKey("articles"), mock.AnythingOfType("*domain.Event")).
			Return("", errors.New("redis error"))

		req := connect.NewRequest(&mqhubv1.PublishRequest{
			Stream: "articles",
			Event: &mqhubv1.Event{
				EventId:   "test-1",
				EventType: "ArticleCreated",
				Source:    "alt-backend",
				CreatedAt: timestamppb.New(time.Now()),
			},
		})

		resp, err := handler.Publish(ctx, req)

		require.Error(t, err)
		assert.False(t, resp.Msg.Success)
		mockPort.AssertExpectations(t)
	})
}

func TestHandler_PublishBatch(t *testing.T) {
	t.Run("publishes batch successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("PublishBatch", ctx, domain.StreamKey("articles"), mock.AnythingOfType("[]*domain.Event")).
			Return([]string{"123-0", "123-1"}, nil)

		req := connect.NewRequest(&mqhubv1.PublishBatchRequest{
			Stream: "articles",
			Events: []*mqhubv1.Event{
				{
					EventId:   "test-1",
					EventType: "ArticleCreated",
					Source:    "alt-backend",
					CreatedAt: timestamppb.New(time.Now()),
				},
				{
					EventId:   "test-2",
					EventType: "ArticleCreated",
					Source:    "alt-backend",
					CreatedAt: timestamppb.New(time.Now()),
				},
			},
		})

		resp, err := handler.PublishBatch(ctx, req)

		require.NoError(t, err)
		assert.Len(t, resp.Msg.MessageIds, 2)
		assert.Equal(t, int32(2), resp.Msg.SuccessCount)
		assert.Equal(t, int32(0), resp.Msg.FailureCount)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when batch publish fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("PublishBatch", ctx, domain.StreamKey("articles"), mock.AnythingOfType("[]*domain.Event")).
			Return(nil, errors.New("redis error"))

		req := connect.NewRequest(&mqhubv1.PublishBatchRequest{
			Stream: "articles",
			Events: []*mqhubv1.Event{
				{
					EventId:   "test-1",
					EventType: "ArticleCreated",
					Source:    "alt-backend",
					CreatedAt: timestamppb.New(time.Now()),
				},
			},
		})

		resp, err := handler.PublishBatch(ctx, req)

		require.Error(t, err)
		assert.Equal(t, int32(0), resp.Msg.SuccessCount)
		assert.Equal(t, int32(1), resp.Msg.FailureCount)
		mockPort.AssertExpectations(t)
	})
}

func TestHandler_CreateConsumerGroup(t *testing.T) {
	t.Run("creates consumer group successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("CreateConsumerGroup", ctx, domain.StreamKey("articles"), domain.ConsumerGroup("pre-processor"), "0").
			Return(nil)

		req := connect.NewRequest(&mqhubv1.CreateConsumerGroupRequest{
			Stream:  "articles",
			Group:   "pre-processor",
			StartId: "0",
		})

		resp, err := handler.CreateConsumerGroup(ctx, req)

		require.NoError(t, err)
		assert.True(t, resp.Msg.Success)
		assert.Equal(t, "consumer group created", resp.Msg.Message)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when create fails", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("CreateConsumerGroup", ctx, domain.StreamKey("articles"), domain.ConsumerGroup("pre-processor"), "0").
			Return(errors.New("stream not found"))

		req := connect.NewRequest(&mqhubv1.CreateConsumerGroupRequest{
			Stream:  "articles",
			Group:   "pre-processor",
			StartId: "0",
		})

		resp, err := handler.CreateConsumerGroup(ctx, req)

		require.Error(t, err)
		assert.False(t, resp.Msg.Success)
		mockPort.AssertExpectations(t)
	})
}

func TestHandler_GetStreamInfo(t *testing.T) {
	t.Run("returns stream info successfully", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()
		expectedInfo := &domain.StreamInfo{
			Length:         100,
			RadixTreeKeys:  5,
			RadixTreeNodes: 10,
			FirstEntryID:   "123-0",
			LastEntryID:    "123-99",
			Groups: []domain.ConsumerGroupInfo{
				{
					Name:            "pre-processor",
					Consumers:       2,
					Pending:         5,
					LastDeliveredID: "123-50",
				},
			},
		}

		mockPort.On("GetStreamInfo", ctx, domain.StreamKey("articles")).Return(expectedInfo, nil)

		req := connect.NewRequest(&mqhubv1.StreamInfoRequest{
			Stream: "articles",
		})

		resp, err := handler.GetStreamInfo(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, int64(100), resp.Msg.Length)
		assert.Equal(t, int64(5), resp.Msg.RadixTreeKeys)
		assert.Equal(t, int64(10), resp.Msg.RadixTreeNodes)
		assert.Equal(t, "123-0", resp.Msg.FirstEntryId)
		assert.Equal(t, "123-99", resp.Msg.LastEntryId)
		assert.Len(t, resp.Msg.Groups, 1)
		assert.Equal(t, "pre-processor", resp.Msg.Groups[0].Name)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns error when stream not found", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("GetStreamInfo", ctx, domain.StreamKey("nonexistent")).
			Return(nil, errors.New("stream not found"))

		req := connect.NewRequest(&mqhubv1.StreamInfoRequest{
			Stream: "nonexistent",
		})

		_, err := handler.GetStreamInfo(ctx, req)

		require.Error(t, err)
		mockPort.AssertExpectations(t)
	})
}

func TestHandler_HealthCheck(t *testing.T) {
	t.Run("returns healthy when Redis is available", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("Ping", ctx).Return(nil)

		req := connect.NewRequest(&mqhubv1.HealthCheckRequest{})

		resp, err := handler.HealthCheck(ctx, req)

		require.NoError(t, err)
		assert.True(t, resp.Msg.Healthy)
		assert.Equal(t, "connected", resp.Msg.RedisStatus)
		mockPort.AssertExpectations(t)
	})

	t.Run("returns unhealthy when Redis is unavailable", func(t *testing.T) {
		mockPort := new(MockStreamPort)
		uc := usecase.NewPublishUsecase(mockPort)
		handler := NewHandler(uc)

		ctx := context.Background()

		mockPort.On("Ping", ctx).Return(errors.New("connection refused"))

		req := connect.NewRequest(&mqhubv1.HealthCheckRequest{})

		resp, err := handler.HealthCheck(ctx, req)

		require.NoError(t, err)
		assert.False(t, resp.Msg.Healthy)
		assert.Equal(t, "connection refused", resp.Msg.RedisStatus)
		mockPort.AssertExpectations(t)
	})
}

func TestProtoEventToDomain(t *testing.T) {
	t.Run("converts proto event to domain event", func(t *testing.T) {
		now := time.Now()
		protoEvent := &mqhubv1.Event{
			EventId:   "test-1",
			EventType: "ArticleCreated",
			Source:    "alt-backend",
			CreatedAt: timestamppb.New(now),
			Payload:   []byte(`{"article_id": "123"}`),
			Metadata:  map[string]string{"trace_id": "abc123"},
		}

		domainEvent := protoEventToDomain(protoEvent)

		assert.Equal(t, "test-1", domainEvent.EventID)
		assert.Equal(t, domain.EventType("ArticleCreated"), domainEvent.EventType)
		assert.Equal(t, "alt-backend", domainEvent.Source)
		assert.Equal(t, now.Unix(), domainEvent.CreatedAt.Unix())
		assert.Equal(t, []byte(`{"article_id": "123"}`), domainEvent.Payload)
		assert.Equal(t, "abc123", domainEvent.Metadata["trace_id"])
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		domainEvent := protoEventToDomain(nil)
		assert.Nil(t, domainEvent)
	})

	t.Run("uses current time when created_at is nil", func(t *testing.T) {
		before := time.Now()
		protoEvent := &mqhubv1.Event{
			EventId:   "test-1",
			EventType: "ArticleCreated",
			Source:    "alt-backend",
			CreatedAt: nil,
		}

		domainEvent := protoEventToDomain(protoEvent)
		after := time.Now()

		assert.True(t, domainEvent.CreatedAt.After(before) || domainEvent.CreatedAt.Equal(before))
		assert.True(t, domainEvent.CreatedAt.Before(after) || domainEvent.CreatedAt.Equal(after))
	})
}
