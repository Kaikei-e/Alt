package gateway

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

type mockStreamDriver struct {
	mock.Mock
}

func (m *mockStreamDriver) Publish(ctx context.Context, stream domain.StreamKey, event *domain.Event) (string, error) {
	args := m.Called(ctx, stream, event)
	return args.String(0), args.Error(1)
}

func (m *mockStreamDriver) PublishBatch(ctx context.Context, stream domain.StreamKey, events []*domain.Event) ([]string, error) {
	args := m.Called(ctx, stream, events)
	if ids := args.Get(0); ids != nil {
		return ids.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) CreateConsumerGroup(ctx context.Context, stream domain.StreamKey, group domain.ConsumerGroup, startID string) error {
	args := m.Called(ctx, stream, group, startID)
	return args.Error(0)
}

func (m *mockStreamDriver) GetStreamInfo(ctx context.Context, stream domain.StreamKey) (*domain.StreamInfo, error) {
	args := m.Called(ctx, stream)
	if info := args.Get(0); info != nil {
		return info.(*domain.StreamInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockStreamDriver) SubscribeWithTimeout(ctx context.Context, stream domain.StreamKey, timeout time.Duration) (*domain.Event, error) {
	args := m.Called(ctx, stream, timeout)
	if event := args.Get(0); event != nil {
		return event.(*domain.Event), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) DeleteStream(ctx context.Context, stream domain.StreamKey) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}

func TestStreamGatewayPublish_RejectsNilEvent(t *testing.T) {
	driver := new(mockStreamDriver)
	gateway := NewStreamGateway(driver)

	_, err := gateway.Publish(context.Background(), domain.StreamKeyArticles, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil event")
	driver.AssertNotCalled(t, "Publish")
}

func TestStreamGatewayPublishBatch_RejectsNilEventInBatch(t *testing.T) {
	driver := new(mockStreamDriver)
	gateway := NewStreamGateway(driver)

	validEvent := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	_, err := gateway.PublishBatch(context.Background(), domain.StreamKeyArticles, []*domain.Event{validEvent, nil})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil event")
	driver.AssertNotCalled(t, "PublishBatch")
}

func TestStreamGatewayPublishBatch_ValidatesBeforeDelegating(t *testing.T) {
	driver := new(mockStreamDriver)
	gateway := NewStreamGateway(driver)

	event := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	driver.On("PublishBatch", mock.Anything, domain.StreamKeyArticles, []*domain.Event{event}).Return([]string{"1-0"}, nil)

	ids, err := gateway.PublishBatch(context.Background(), domain.StreamKeyArticles, []*domain.Event{event})

	require.NoError(t, err)
	assert.Equal(t, []string{"1-0"}, ids)
	driver.AssertExpectations(t)
}

func TestStreamGatewayPublish_DelegatesDriverError(t *testing.T) {
	driver := new(mockStreamDriver)
	gateway := NewStreamGateway(driver)

	event := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	driver.On("Publish", mock.Anything, domain.StreamKeyArticles, event).Return("", errors.New("redis down"))

	_, err := gateway.Publish(context.Background(), domain.StreamKeyArticles, event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis down")
	driver.AssertExpectations(t)
}
