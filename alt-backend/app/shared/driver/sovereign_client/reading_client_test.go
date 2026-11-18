package sovereign_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

type fakeReadingSovereignServer struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	queueItems []*sovereignv1.ReadingQueueEntry
	readCount  int32
}

func (f *fakeReadingSovereignServer) GetReadingQueue(
	_ context.Context,
	_ *connect.Request[sovereignv1.GetReadingQueueRequest],
) (*connect.Response[sovereignv1.GetReadingQueueResponse], error) {
	return connect.NewResponse(&sovereignv1.GetReadingQueueResponse{Items: f.queueItems}), nil
}

func (f *fakeReadingSovereignServer) CountReadingSourcesSince(
	_ context.Context,
	_ *connect.Request[sovereignv1.CountReadingSourcesSinceRequest],
) (*connect.Response[sovereignv1.CountReadingSourcesSinceResponse], error) {
	return connect.NewResponse(&sovereignv1.CountReadingSourcesSinceResponse{Count: f.readCount}), nil
}

func TestClient_GetReadingQueueAndCountReadingSourcesSince(t *testing.T) {
	articleID := uuid.New()
	queuedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	expiresAt := queuedAt.Add(48 * time.Hour)

	srv := &fakeReadingSovereignServer{
		queueItems: []*sovereignv1.ReadingQueueEntry{{
			ArticleId:          articleID.String(),
			QueuedAt:           timestamppb.New(queuedAt),
			ExpiresAt:          timestamppb.New(expiresAt),
			QueuePolicyVersion: 1,
		}},
		readCount: 4,
	}
	mux := http.NewServeMux()
	path, handler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(srv)
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := NewClient(ts.URL, true)
	userID := uuid.New()

	entries, err := c.GetReadingQueue(context.Background(), userID, queuedAt.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, articleID, entries[0].ArticleID)
	assert.Equal(t, queuedAt, entries[0].QueuedAt)
	assert.Equal(t, expiresAt, entries[0].ExpiresAt)
	assert.Equal(t, 1, entries[0].QueuePolicyVersion)

	cnt, err := c.CountReadingSourcesSince(context.Background(), userID, queuedAt)
	require.NoError(t, err)
	assert.Equal(t, 4, cnt)
}
