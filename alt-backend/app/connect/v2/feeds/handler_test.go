package feeds

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/di"
	"alt/domain"
)

// mockStreamServer implements connect.ServerStream for testing
type mockStreamServer struct {
	messages []*feedsv2.StreamFeedStatsResponse
	ctx      context.Context
}

func (m *mockStreamServer) Send(msg *feedsv2.StreamFeedStatsResponse) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockStreamServer) Conn() *connect.StreamingHandlerConn {
	return nil
}

func (m *mockStreamServer) RequestHeader() connect.Header {
	return make(connect.Header)
}

func (m *mockStreamServer) Receive(msg interface{}) error {
	return io.EOF
}

// mockUsecase provides predictable test data
type mockFeedAmountUsecase struct{}

func (m *mockFeedAmountUsecase) Execute(ctx context.Context) (int, error) {
	return 10, nil
}

type mockSummarizedArticlesUsecase struct{}

func (m *mockSummarizedArticlesUsecase) Execute(ctx context.Context) (int, error) {
	return 7, nil
}

type mockTotalArticlesUsecase struct{}

func (m *mockTotalArticlesUsecase) Execute(ctx context.Context) (int, error) {
	return 100, nil
}

type mockUnsummarizedArticlesUsecase struct{}

func (m *mockUnsummarizedArticlesUsecase) Execute(ctx context.Context) (int, error) {
	return 3, nil
}

// createTestHandler creates a handler with mock usecases
func createTestHandler() *Handler {
	container := &di.ApplicationComponents{
		FeedAmountUsecase:              &mockFeedAmountUsecase{},
		SummarizedArticlesCountUsecase: &mockSummarizedArticlesUsecase{},
		TotalArticlesCountUsecase:      &mockTotalArticlesUsecase{},
		UnsummarizedArticlesCountUsecase: &mockUnsummarizedArticlesUsecase{},
	}

	logger := slog.Default()

	return NewHandler(container, logger)
}

func TestStreamFeedStats_Authentication(t *testing.T) {
	handler := createTestHandler()

	// Create context without authentication
	ctx := context.Background()

	req := connect.NewRequest(&feedsv2.StreamFeedStatsRequest{})
	stream := &mockStreamServer{ctx: ctx}

	// Should fail because no user context
	err := handler.StreamFeedStats(ctx, req, stream)

	require.Error(t, err)
	connectErr := err.(*connect.Error)
	assert.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestStreamFeedStats_SendsInitialData(t *testing.T) {
	handler := createTestHandler()

	// Create authenticated context
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:  "test-user",
		ActorID: "test-actor",
	})

	// Create context with timeout to prevent indefinite blocking
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	req := connect.NewRequest(&feedsv2.StreamFeedStatsRequest{})
	stream := &mockStreamServer{ctx: ctx}

	// Start streaming in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.StreamFeedStats(ctx, req, stream)
	}()

	// Wait for context cancellation or error
	select {
	case err := <-errCh:
		// Context cancellation is expected
		if err != nil && ctx.Err() != context.Canceled && ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("stream did not complete in time")
	}

	// Should have sent at least initial data
	require.Greater(t, len(stream.messages), 0, "should send at least one message")

	firstMsg := stream.messages[0]
	assert.Equal(t, int64(10), firstMsg.FeedAmount)
	assert.Equal(t, int64(3), firstMsg.UnsummarizedFeedAmount)
	assert.Equal(t, int64(100), firstMsg.TotalArticles)
	assert.NotNil(t, firstMsg.Metadata)
	assert.False(t, firstMsg.Metadata.IsHeartbeat)
	assert.Greater(t, firstMsg.Metadata.Timestamp, int64(0))
}

func TestStreamFeedStats_SendsHeartbeat(t *testing.T) {
	handler := createTestHandler()

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:  "test-user",
		ActorID: "test-actor",
	})

	// Use longer timeout to allow heartbeat
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req := connect.NewRequest(&feedsv2.StreamFeedStatsRequest{})
	stream := &mockStreamServer{ctx: ctx}

	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.StreamFeedStats(ctx, req, stream)
	}()

	// Wait enough time for heartbeat (should happen at 10s interval)
	time.Sleep(11 * time.Second)
	cancel()

	<-errCh

	// Should have multiple messages (initial + updates + heartbeat)
	require.Greater(t, len(stream.messages), 1, "should send multiple messages")

	// Check if at least one heartbeat was sent
	hasHeartbeat := false
	for _, msg := range stream.messages {
		if msg.Metadata != nil && msg.Metadata.IsHeartbeat {
			hasHeartbeat = true
			// Heartbeat should have zero stats
			assert.Equal(t, int64(0), msg.FeedAmount)
			assert.Equal(t, int64(0), msg.UnsummarizedFeedAmount)
			assert.Equal(t, int64(0), msg.TotalArticles)
			break
		}
	}
	assert.True(t, hasHeartbeat, "should send at least one heartbeat")
}

func TestStreamFeedStats_RespectsContextCancellation(t *testing.T) {
	handler := createTestHandler()

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:  "test-user",
		ActorID: "test-actor",
	})

	ctx, cancel := context.WithCancel(ctx)

	req := connect.NewRequest(&feedsv2.StreamFeedStatsRequest{})
	stream := &mockStreamServer{ctx: ctx}

	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.StreamFeedStats(ctx, req, stream)
	}()

	// Wait a bit to ensure streaming started
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Should return promptly
	select {
	case err := <-errCh:
		// Context cancellation should return nil (graceful shutdown)
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not respond to context cancellation")
	}
}
