package sovereign_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWatchHandler struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	mu            sync.Mutex
	authHeader    string
	projectorName string
}

func (m *mockWatchHandler) WatchProjectorEvents(
	_ context.Context,
	req *connect.Request[sovereignv1.WatchProjectorEventsRequest],
	stream *connect.ServerStream[sovereignv1.WatchProjectorEventsResponse],
) error {
	m.mu.Lock()
	m.authHeader = req.Header().Get("Authorization")
	m.projectorName = req.Msg.ProjectorName
	m.mu.Unlock()

	return stream.Send(&sovereignv1.WatchProjectorEventsResponse{
		LatestEventSeq: 42,
	})
}

func TestConnectProjectorWatch_SendsAuthorizationHeader(t *testing.T) {
	handler := &mockWatchHandler{}
	mux := http.NewServeMux()
	path, h := sovereignv1connect.NewKnowledgeSovereignServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, true, WithEventToken("test-watch-token-secret-1234"))
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	listener, err := client.ConnectProjectorWatch(ctx, "today_digest_view")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, listener.Close(context.Background()))
	}()

	err = listener.WaitForNotification(ctx)
	require.NoError(t, err)

	handler.mu.Lock()
	defer handler.mu.Unlock()
	assert.Equal(t, "Bearer test-watch-token-secret-1234", handler.authHeader)
	assert.Equal(t, "today_digest_view", handler.projectorName)
}
