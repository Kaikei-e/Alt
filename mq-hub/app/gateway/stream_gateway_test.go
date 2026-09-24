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
	"mq-hub/driver"
)

type mockStreamDriver struct {
	mock.Mock
}

func (m *mockStreamDriver) Publish(ctx context.Context, stream string, msg *driver.StreamMessage) (string, error) {
	args := m.Called(ctx, stream, msg)
	return args.String(0), args.Error(1)
}

func (m *mockStreamDriver) PublishBatch(ctx context.Context, stream string, msgs []*driver.StreamMessage) ([]string, error) {
	args := m.Called(ctx, stream, msgs)
	if ids := args.Get(0); ids != nil {
		return ids.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) CreateConsumerGroup(ctx context.Context, stream, group, startID string) error {
	args := m.Called(ctx, stream, group, startID)
	return args.Error(0)
}

func (m *mockStreamDriver) GetStreamInfo(ctx context.Context, stream string) (*driver.StreamInfo, error) {
	args := m.Called(ctx, stream)
	if info := args.Get(0); info != nil {
		return info.(*driver.StreamInfo), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockStreamDriver) SubscribeWithTimeout(ctx context.Context, stream string, timeout time.Duration) (*driver.StreamMessage, error) {
	args := m.Called(ctx, stream, timeout)
	if msg := args.Get(0); msg != nil {
		return msg.(*driver.StreamMessage), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) DeleteStream(ctx context.Context, stream string) error {
	args := m.Called(ctx, stream)
	return args.Error(0)
}

func (m *mockStreamDriver) Expire(ctx context.Context, stream string, ttl time.Duration) error {
	args := m.Called(ctx, stream, ttl)
	return args.Error(0)
}

func (m *mockStreamDriver) ScanReplyStreamsWithoutTTL(ctx context.Context, prefix string) ([]string, error) {
	args := m.Called(ctx, prefix)
	if keys := args.Get(0); keys != nil {
		return keys.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockStreamDriver) TrimMaxLenApprox(ctx context.Context, stream string, maxLen int64) (int64, error) {
	args := m.Called(ctx, stream, maxLen)
	return args.Get(0).(int64), args.Error(1)
}

func TestStreamGatewayPublish_RejectsNilEvent(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	_, err := gw.Publish(context.Background(), domain.StreamKeyArticles, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil event")
	drv.AssertNotCalled(t, "Publish")
}

func TestStreamGatewayPublish_RejectsInvalidEvent(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	// Missing EventID fails domain.Event.Validate(); the gateway must catch
	// this before ever reaching the driver, not just reject nil events.
	invalidEvent := &domain.Event{
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	_, err := gw.Publish(context.Background(), domain.StreamKeyArticles, invalidEvent)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "event_id is required")
	drv.AssertNotCalled(t, "Publish")
}

func TestStreamGatewayPublishBatch_RejectsInvalidEventInBatch(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	validEvent := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}
	invalidEvent := &domain.Event{
		EventID:   "evt-2",
		EventType: domain.EventTypeArticleCreated,
		CreatedAt: time.Now(),
	}

	_, err := gw.PublishBatch(context.Background(), domain.StreamKeyArticles, []*domain.Event{validEvent, invalidEvent})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "source is required")
	drv.AssertNotCalled(t, "PublishBatch")
}

func TestStreamGatewayPublishBatch_RejectsNilEventInBatch(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	validEvent := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	_, err := gw.PublishBatch(context.Background(), domain.StreamKeyArticles, []*domain.Event{validEvent, nil})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil event")
	drv.AssertNotCalled(t, "PublishBatch")
}

func TestStreamGatewayPublishBatch_ValidatesBeforeDelegating(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	event := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	drv.On("PublishBatch", mock.Anything, domain.StreamKeyArticles.String(), mock.MatchedBy(func(msgs []*driver.StreamMessage) bool {
		return len(msgs) == 1 && msgs[0].EventID == "evt-1"
	})).Return([]string{"1-0"}, nil)

	ids, err := gw.PublishBatch(context.Background(), domain.StreamKeyArticles, []*domain.Event{event})

	require.NoError(t, err)
	assert.Equal(t, []string{"1-0"}, ids)
	drv.AssertExpectations(t)
}

func TestStreamGatewayPublish_DelegatesDriverError(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	event := &domain.Event{
		EventID:   "evt-1",
		EventType: domain.EventTypeArticleCreated,
		Source:    "alt-backend",
		CreatedAt: time.Now(),
	}

	drv.On("Publish", mock.Anything, domain.StreamKeyArticles.String(), mock.Anything).Return("", errors.New("redis down"))

	_, err := gw.Publish(context.Background(), domain.StreamKeyArticles, event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis down")
	drv.AssertExpectations(t)
}

func TestStreamGatewayExpire_DelegatesToDriver(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	drv.On("Expire", mock.Anything, domain.StreamKeyArticles.String(), 5*time.Minute).Return(nil)

	err := gw.Expire(context.Background(), domain.StreamKeyArticles, 5*time.Minute)

	require.NoError(t, err)
	drv.AssertExpectations(t)
}

func TestStreamGatewaySubscribeWithTimeout_MapsDriverMessageToDomainEvent(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	now := time.Now().UTC().Truncate(time.Millisecond)
	msg := &driver.StreamMessage{
		ID:        "123-0",
		EventID:   "evt-reply",
		EventType: "TagGenerationCompleted",
		Source:    "tag-generator",
		CreatedAt: now.Format(time.RFC3339),
		Payload:   `{"done":true}`,
		Metadata:  `{"correlation_id":"corr-123"}`,
	}

	drv.On("SubscribeWithTimeout", mock.Anything, "alt:replies:tags:corr-123", time.Second).Return(msg, nil)

	event, err := gw.SubscribeWithTimeout(context.Background(), domain.StreamKey("alt:replies:tags:corr-123"), time.Second)

	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, "evt-reply", event.EventID)
	assert.Equal(t, domain.EventTypeTagGenerationCompleted, event.EventType)
	assert.Equal(t, "corr-123", event.Metadata["correlation_id"])
	drv.AssertExpectations(t)
}

func TestStreamGatewaySubscribeWithTimeout_MapsTimeoutError(t *testing.T) {
	drv := new(mockStreamDriver)
	gw := NewStreamGateway(drv)

	drv.On("SubscribeWithTimeout", mock.Anything, "alt:replies:tags:corr-123", time.Second).Return(nil, driver.ErrReplyTimeout)

	event, err := gw.SubscribeWithTimeout(context.Background(), domain.StreamKey("alt:replies:tags:corr-123"), time.Second)

	require.Nil(t, event)
	require.ErrorIs(t, err, domain.ErrReplyTimeout)
	drv.AssertExpectations(t)
}
