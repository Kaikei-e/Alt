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
		require.Len(t, resp.Msg.GetItems(), 1)
		assert.Equal(t, articleID.String(), resp.Msg.GetItems()[0].GetArticleId())
		assert.Equal(t, queuedAt, resp.Msg.GetItems()[0].GetQueuedAt().AsTime())
		assert.Equal(t, expiresAt, resp.Msg.GetItems()[0].GetExpiresAt().AsTime())
		assert.Equal(t, int32(1), resp.Msg.GetItems()[0].GetQueuePolicyVersion())
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
		assert.Equal(t, int32(5), resp.Msg.GetCount())
	})
}

func TestListReadingSourcesInWindow_ValidAndInvalidRequests(t *testing.T) {
	userID := uuid.New()
	artV1 := uuid.New()
	artLegacy := uuid.New()
	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	readAtV1 := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	readAtLegacy := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		readingSourcesInWindow: []sovereign_db.ReadingSource{
			{UserID: userID, ArticleID: artV1, FirstReadAt: readAtV1, ReadRuleVersion: 1},
			{UserID: userID, ArticleID: artLegacy, FirstReadAt: readAtLegacy, ReadRuleVersion: 0},
		},
	}
	h := NewSovereignHandler(repo)

	t.Run("rejects invalid user_id", func(t *testing.T) {
		_, err := h.ListReadingSourcesInWindow(context.Background(), connect.NewRequest(&sovereignv1.ListReadingSourcesInWindowRequest{
			UserId: "bad",
			From:   timestamppb.New(from),
			To:     timestamppb.New(to),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("rejects missing from", func(t *testing.T) {
		_, err := h.ListReadingSourcesInWindow(context.Background(), connect.NewRequest(&sovereignv1.ListReadingSourcesInWindowRequest{
			UserId: userID.String(),
			To:     timestamppb.New(to),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("rejects missing to", func(t *testing.T) {
		_, err := h.ListReadingSourcesInWindow(context.Background(), connect.NewRequest(&sovereignv1.ListReadingSourcesInWindowRequest{
			UserId: userID.String(),
			From:   timestamppb.New(from),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("rejects to before from", func(t *testing.T) {
		_, err := h.ListReadingSourcesInWindow(context.Background(), connect.NewRequest(&sovereignv1.ListReadingSourcesInWindowRequest{
			UserId: userID.String(),
			From:   timestamppb.New(to),
			To:     timestamppb.New(from),
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("returns all versions preserving rule version", func(t *testing.T) {
		resp, err := h.ListReadingSourcesInWindow(context.Background(), connect.NewRequest(&sovereignv1.ListReadingSourcesInWindowRequest{
			UserId: userID.String(),
			From:   timestamppb.New(from),
			To:     timestamppb.New(to),
		}))
		require.NoError(t, err)
		require.Len(t, resp.Msg.GetEntries(), 2)
		assert.Equal(t, artV1.String(), resp.Msg.GetEntries()[0].GetArticleId())
		assert.Equal(t, readAtV1, resp.Msg.GetEntries()[0].GetFirstReadAt().AsTime())
		assert.Equal(t, int32(1), resp.Msg.GetEntries()[0].GetReadRuleVersion())
		assert.Equal(t, artLegacy.String(), resp.Msg.GetEntries()[1].GetArticleId())
		assert.Equal(t, readAtLegacy, resp.Msg.GetEntries()[1].GetFirstReadAt().AsTime())
		assert.Equal(t, int32(0), resp.Msg.GetEntries()[1].GetReadRuleVersion())
	})
}
