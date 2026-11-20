package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"pre-processor/domain"
	"pre-processor/test/mocks"
)

type stubSummarizeService struct {
	called    bool
	articleID string
	title     string
	origin    domain.SummarizeOrigin
	err       error
}

func (s *stubSummarizeService) SummarizeArticle(_ context.Context, articleID, title string, origin domain.SummarizeOrigin) error {
	s.called = true
	s.articleID = articleID
	s.title = title
	s.origin = origin
	return s.err
}

func TestPreProcessorEventHandler_OriginRouting(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("ArticleCreated enqueues with system origin", func(t *testing.T) {
		svc := &stubSummarizeService{}
		h := NewPreProcessorEventHandler(svc, logger)

		err := h.HandleEvent(context.Background(), Event{
			EventID:   "evt-1",
			EventType: EventTypeArticleCreated,
			Payload:   json.RawMessage(`{"article_id":"art-1","title":"Title 1"}`),
		})

		require.NoError(t, err)
		require.True(t, svc.called)
		assert.Equal(t, "art-1", svc.articleID)
		assert.Equal(t, "Title 1", svc.title)
		assert.Equal(t, domain.SummarizeOriginSystem, svc.origin)
	})

	t.Run("SummarizeRequested forwards valid user and system origins", func(t *testing.T) {
		for _, wantOrigin := range []domain.SummarizeOrigin{domain.SummarizeOriginUser, domain.SummarizeOriginSystem} {
			svc := &stubSummarizeService{}
			h := NewPreProcessorEventHandler(svc, logger)

			payload, err := json.Marshal(SummarizeRequestedPayload{
				ArticleID: "art-2",
				UserID:    "user-1",
				Title:     "Title 2",
				Origin:    string(wantOrigin),
			})
			require.NoError(t, err)

			err = h.HandleEvent(context.Background(), Event{
				EventID:   "evt-2",
				EventType: EventTypeSummarizeRequested,
				Payload:   payload,
			})

			require.NoError(t, err)
			require.True(t, svc.called)
			assert.Equal(t, "art-2", svc.articleID)
			assert.Equal(t, "Title 2", svc.title)
			assert.Equal(t, wantOrigin, svc.origin)
		}
	})

	t.Run("SummarizeRequested rejects missing or invalid origin without calling service", func(t *testing.T) {
		rawPayloads := []string{
			`{"article_id":"art-3","title":"Title 3"}`,
			`{"article_id":"art-3","title":"Title 3","origin":""}`,
			`{"article_id":"art-3","title":"Title 3","origin":"unknown"}`,
			`{"article_id":"art-3","title":"Title 3","origin":"USER"}`,
		}

		for _, raw := range rawPayloads {
			svc := &stubSummarizeService{}
			h := NewPreProcessorEventHandler(svc, logger)

			err := h.HandleEvent(context.Background(), Event{
				EventID:   "evt-3",
				EventType: EventTypeSummarizeRequested,
				Payload:   json.RawMessage(raw),
			})

			require.ErrorIs(t, err, domain.ErrInvalidSummarizeOrigin, "payload: %s", raw)
			assert.False(t, svc.called, "SummarizeArticle must not be called for payload: %s", raw)
		}
	})
}

func TestSummarizeServiceAdapter_OriginPassingAndValidation(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("passes explicit origin to CreateJob", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		articleRepo := mocks.NewMockArticleRepository(ctrl)
		summaryRepo := mocks.NewMockSummaryRepository(ctrl)

		summaryRepo.EXPECT().Exists(gomock.Any(), "art-10").Return(false, nil)
		jobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "art-10", gomock.Any()).Return(false, nil)
		jobRepo.EXPECT().HasInFlightJob(gomock.Any(), "art-10", gomock.Any()).Return(false, nil)
		jobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "art-10").Return(false, nil)
		jobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), "art-10", gomock.Any()).Return(false, nil)
		jobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), "art-10").Return(0, nil)
		jobRepo.EXPECT().CreateJob(gomock.Any(), "art-10", domain.SummarizeOriginSystem).Return("job-10", nil)

		adapter := NewSummarizeServiceAdapter(jobRepo, articleRepo, summaryRepo, logger)
		err := adapter.SummarizeArticle(context.Background(), "art-10", "Title", domain.SummarizeOriginSystem)
		require.NoError(t, err)
	})

	t.Run("rejects invalid origin before guard check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		articleRepo := mocks.NewMockArticleRepository(ctrl)
		summaryRepo := mocks.NewMockSummaryRepository(ctrl)

		adapter := NewSummarizeServiceAdapter(jobRepo, articleRepo, summaryRepo, logger)
		err := adapter.SummarizeArticle(context.Background(), "art-10", "Title", "")
		require.ErrorIs(t, err, domain.ErrInvalidSummarizeOrigin)
	})
}

func TestConsumer_SummarizeRequestedWithoutOriginLeavesMessageUnackedAndRoutesToDLQ(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	svc := &stubSummarizeService{}
	handler := NewPreProcessorEventHandler(svc, logger)

	cfg := DefaultConfig()
	cfg.RedisURL = "redis://" + mr.Addr()
	cfg.Enabled = true
	cfg.ClaimIdleTime = 10 * time.Millisecond
	cfg.MaxDeliveries = 1

	c, err := NewConsumer(cfg, handler, logger)
	require.NoError(t, err)
	defer func() { _ = c.client.Close() }()

	require.NoError(t, c.ensureConsumerGroup(ctx))

	msgID, err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg.StreamKey,
		Values: map[string]any{
			"event_id":   "evt-missing-origin",
			"event_type": EventTypeSummarizeRequested,
			"source":     "test",
			"payload":    `{"article_id":"art-no-origin","title":"Title"}`,
		},
	}).Result()
	require.NoError(t, err)

	require.NoError(t, c.readAndProcess(ctx))
	assert.False(t, svc.called)

	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamKey,
		Group:  cfg.GroupName,
		Start:  msgID,
		End:    msgID,
		Count:  1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, pending, 1, "failed message must remain un-ACKed in the PEL")

	virtualNow := time.Now()
	for range 3 {
		virtualNow = virtualNow.Add(cfg.ClaimIdleTime + time.Second)
		mr.SetTime(virtualNow)
		require.NoError(t, c.reclaimPending(ctx))
	}

	pendingAfterDLQ, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamKey,
		Group:  cfg.GroupName,
		Start:  msgID,
		End:    msgID,
		Count:  1,
	}).Result()
	require.NoError(t, err)
	assert.Empty(t, pendingAfterDLQ, "message should be ACKed after durable DLQ write")

	dlqMsgs, err := c.client.XRange(ctx, cfg.DLQStreamKey, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, dlqMsgs, 1, "poison message without origin must land in DLQ after exceeding MaxDeliveries")
	assert.Equal(t, msgID, dlqMsgs[0].Values["dlq_original_id"])
}
