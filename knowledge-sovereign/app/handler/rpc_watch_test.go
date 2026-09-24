package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
)

type mockNotification struct {
	payload   string
	isTimeout bool
}

type mockProjectorWatcher struct {
	notifications []mockNotification
	index         int
	closed        atomic.Bool
}

func (m *mockProjectorWatcher) WaitForNotification(ctx context.Context, timeout time.Duration) (string, bool, error) {
	if m.index >= len(m.notifications) {
		<-ctx.Done()
		return "", false, ctx.Err()
	}
	n := m.notifications[m.index]
	m.index++
	return n.payload, n.isTimeout, nil
}

func (m *mockProjectorWatcher) Close(ctx context.Context) error {
	m.closed.Store(true)
	return nil
}

func TestWatchProjectorEvents_DatabaseURLNotConfigured(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	mux := http.NewServeMux()
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h)
	mux.Handle(path, rpcHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	stream, err := client.WatchProjectorEvents(context.Background(), connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
		ProjectorName: "test_projector",
	}))
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	assert.False(t, stream.Receive())
	require.Error(t, stream.Err())
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(stream.Err()))
}

func TestWatchProjectorEvents_StreamsNotificationsAndHeartbeats(t *testing.T) {
	watcher := &mockProjectorWatcher{
		notifications: []mockNotification{
			{payload: "101", isTimeout: false},
			{payload: "", isTimeout: true}, // simulates heartbeat interval timeout
			{payload: "102", isTimeout: false},
		},
	}

	repo := &mockRepo{}
	h := NewSovereignHandler(
		repo,
		WithDatabaseURL("postgres://mock"),
		WithProjectorWatcherOpener(func(ctx context.Context, databaseURL string) (ProjectorWatcher, error) {
			return watcher, nil
		}),
	)

	mux := http.NewServeMux()
	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(h)
	mux.Handle(path, rpcHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := sovereignv1connect.NewKnowledgeSovereignServiceClient(srv.Client(), srv.URL)
	stream, err := client.WatchProjectorEvents(ctx, connect.NewRequest(&sovereignv1.WatchProjectorEventsRequest{
		ProjectorName: "test_projector",
	}))
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	// First event: seq 101
	require.True(t, stream.Receive())
	assert.Equal(t, int64(101), stream.Msg().LatestEventSeq)

	// Second event: heartbeat (seq 0)
	require.True(t, stream.Receive())
	assert.Equal(t, int64(0), stream.Msg().LatestEventSeq)

	// Third event: seq 102
	require.True(t, stream.Receive())
	assert.Equal(t, int64(102), stream.Msg().LatestEventSeq)

	// Cancel context to trigger clean client disconnect
	cancel()

	// Stream should finish
	for stream.Receive() {
	}
	assert.True(t, errors.Is(stream.Err(), context.Canceled), "expected context.Canceled on client stream after cancel")

	// Ensure the watcher Close was executed on the server
	assert.Eventually(t, func() bool {
		return watcher.closed.Load()
	}, 1*time.Second, 10*time.Millisecond)
}
