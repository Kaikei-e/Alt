package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestGetReadingQueue_ValidAndInvalidRequests(t *testing.T) {
	userID := uuid.New()
	articleID := uuid.New()
	asOf := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	queuedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	expiresAt := queuedAt.Add(48 * time.Hour)

	repo := &mockRepo{
		readingQueueItems: []sovereign_db.ReadingQueueItem{{
			UserID:             userID,
			ArticleID:          articleID,
			QueuedAt:           queuedAt,
			ExpiresAt:          expiresAt,
			QueuePolicyVersion: 1,
		}},
	}
	h := NewSovereignHandler(repo)

	t.Run("rejects invalid user_id", func(t *testing.T) {
		_, err := h.GetReadingQueue(context.Background(), connect.NewRequest(&sovereignv1.GetReadingQueueRequest{
			UserId: "bad-uuid",
			AsOf:   timestamppb.New(asOf),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("rejects missing as_of", func(t *testing.T) {
		_, err := h.GetReadingQueue(context.Background(), connect.NewRequest(&sovereignv1.GetReadingQueueRequest{
			UserId: userID.String(),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("returns mapped active entries", func(t *testing.T) {
		resp, err := h.GetReadingQueue(context.Background(), connect.NewRequest(&sovereignv1.GetReadingQueueRequest{
			UserId: userID.String(),
			AsOf:   timestamppb.New(asOf),
		}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.Items, 1)
		assert.Equal(t, articleID.String(), resp.Msg.Items[0].ArticleId)
		assert.Equal(t, queuedAt, resp.Msg.Items[0].QueuedAt.AsTime())
		assert.Equal(t, expiresAt, resp.Msg.Items[0].ExpiresAt.AsTime())
		assert.Equal(t, int32(1), resp.Msg.Items[0].QueuePolicyVersion)
	})
}

func TestCountReadingSourcesSince_ValidAndInvalidRequests(t *testing.T) {
	userID := uuid.New()
	since := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	repo := &mockRepo{readingSourcesCount: 5}
	h := NewSovereignHandler(repo)

	t.Run("rejects invalid user_id", func(t *testing.T) {
		_, err := h.CountReadingSourcesSince(context.Background(), connect.NewRequest(&sovereignv1.CountReadingSourcesSinceRequest{
			UserId: "bad",
			Since:  timestamppb.New(since),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("rejects missing since", func(t *testing.T) {
		_, err := h.CountReadingSourcesSince(context.Background(), connect.NewRequest(&sovereignv1.CountReadingSourcesSinceRequest{
			UserId: userID.String(),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("returns count", func(t *testing.T) {
		resp, err := h.CountReadingSourcesSince(context.Background(), connect.NewRequest(&sovereignv1.CountReadingSourcesSinceRequest{
			UserId: userID.String(),
			Since:  timestamppb.New(since),
		}))
		require.NoError(t, err)
		assert.Equal(t, int32(5), resp.Msg.Count)
	})
}
