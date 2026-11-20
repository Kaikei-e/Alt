package feeds

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/feed_inflow_stats_usecase"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/orchestrator/usecase/subscription_usecase"
	"alt/utils/logger"
)

type stubSubscriptionCapPort struct {
	subscriptions map[uuid.UUID]map[uuid.UUID]*int // userID -> feedLinkID -> dailyCap
	err           error
}

func (s *stubSubscriptionCapPort) ListSubscriptions(context.Context, uuid.UUID) ([]*domain.FeedSource, error) {
	return nil, nil
}
func (s *stubSubscriptionCapPort) Subscribe(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *stubSubscriptionCapPort) Unsubscribe(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *stubSubscriptionCapPort) SetSubscriptionDailyCap(_ context.Context, userID uuid.UUID, feedLinkID uuid.UUID, dailyCap *int) error {
	if s.err != nil {
		return s.err
	}
	userSubs, ok := s.subscriptions[userID]
	if !ok {
		return domain.ErrFeedNotFound
	}
	if _, exists := userSubs[feedLinkID]; !exists {
		return domain.ErrFeedNotFound
	}
	userSubs[feedLinkID] = dailyCap
	return nil
}

type stubFeedInflowReadPort struct {
	inflows           []domain.FeedInflow
	articleToFeedLink map[uuid.UUID]uuid.UUID
	err               error
}

func (s *stubFeedInflowReadPort) GetFeedInflowByUser(context.Context, uuid.UUID, time.Time, time.Time) ([]domain.FeedInflow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.inflows, nil
}

func (s *stubFeedInflowReadPort) BatchGetFeedLinkIDsByArticleIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.articleToFeedLink, nil
}

type stubReadingSourcesWindowPort struct {
	entries []domain.ReadingSourceEntry
	err     error
}

func (s *stubReadingSourcesWindowPort) ListReadingSourcesInWindow(context.Context, uuid.UUID, time.Time, time.Time) ([]domain.ReadingSourceEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

func authCtxForUser(userID uuid.UUID) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestSetSubscriptionDailyCap(t *testing.T) {
	logger.InitLogger()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	ownedLinkID := uuid.New()
	otherLinkID := uuid.New()

	newSubPort := func() *stubSubscriptionCapPort {
		return &stubSubscriptionCapPort{
			subscriptions: map[uuid.UUID]map[uuid.UUID]*int{
				ownerID:     {ownedLinkID: nil},
				otherUserID: {otherLinkID: nil},
			},
		}
	}

	t.Run("requires authentication", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			SetSubscriptionDailyCap: subscription_usecase.NewSetSubscriptionDailyCapUsecase(newSubPort()),
			ResolveArticle:          resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		_, err := h.SetSubscriptionDailyCap(context.Background(), connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: ownedLinkID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("rejects invalid feed_link_id and non-positive daily_cap", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			SetSubscriptionDailyCap: subscription_usecase.NewSetSubscriptionDailyCapUsecase(newSubPort()),
			ResolveArticle:          resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())
		ctx := authCtxForUser(ownerID)

		_, err := h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: "",
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		_, err = h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: "not-a-uuid",
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

		zero := int32(0)
		_, err = h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: ownedLinkID.String(),
			DailyCap:   &zero,
		}))
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("sets and clears cap for caller's subscription", func(t *testing.T) {
		subPort := newSubPort()
		h := NewHandler(FeedHandlerDeps{
			SetSubscriptionDailyCap: subscription_usecase.NewSetSubscriptionDailyCapUsecase(subPort),
			ResolveArticle:          resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())
		ctx := authCtxForUser(ownerID)

		capVal := int32(2)
		resp, err := h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: ownedLinkID.String(),
			DailyCap:   &capVal,
		}))
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.DailyCap)
		assert.Equal(t, int32(2), resp.Msg.GetDailyCap())
		require.NotNil(t, subPort.subscriptions[ownerID][ownedLinkID])
		assert.Equal(t, 2, *subPort.subscriptions[ownerID][ownedLinkID])

		clearResp, err := h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: ownedLinkID.String(),
			DailyCap:   nil,
		}))
		require.NoError(t, err)
		assert.Nil(t, clearResp.Msg.DailyCap)
		assert.Nil(t, subPort.subscriptions[ownerID][ownedLinkID])
	})

	t.Run("rejects setting cap on someone else's subscription", func(t *testing.T) {
		subPort := newSubPort()
		h := NewHandler(FeedHandlerDeps{
			SetSubscriptionDailyCap: subscription_usecase.NewSetSubscriptionDailyCapUsecase(subPort),
			ResolveArticle:          resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())
		ctx := authCtxForUser(ownerID)

		capVal := int32(4)
		_, err := h.SetSubscriptionDailyCap(ctx, connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
			FeedLinkId: otherLinkID.String(),
			DailyCap:   &capVal,
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
		assert.Nil(t, subPort.subscriptions[otherUserID][otherLinkID])
	})

	t.Run("panics when usecase is unwired", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			ResolveArticle: resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		assert.Panics(t, func() {
			_, _ = h.SetSubscriptionDailyCap(authCtxForUser(ownerID), connect.NewRequest(&feedsv2.SetSubscriptionDailyCapRequest{
				FeedLinkId: ownedLinkID.String(),
			}))
		})
	})
}

func TestGetFeedInflowStats(t *testing.T) {
	logger.InitLogger()
	userID := uuid.New()
	linkA := uuid.New()
	artRead := uuid.New()
	artLegacy := uuid.New()

	t.Run("requires authentication", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			FeedInflowStats: feed_inflow_stats_usecase.NewFeedInflowStatsUsecase(&stubFeedInflowReadPort{}, &stubReadingSourcesWindowPort{}),
			ResolveArticle:  resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		_, err := h.GetFeedInflowStats(context.Background(), connect.NewRequest(&feedsv2.GetFeedInflowStatsRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("never counts version-0 row in read_count", func(t *testing.T) {
		capVal := 5
		inflowPort := &stubFeedInflowReadPort{
			inflows: []domain.FeedInflow{
				{FeedLinkID: linkA, FeedURL: "https://example.com/a.xml", DailyCap: &capVal, InflowCount: 10},
			},
			articleToFeedLink: map[uuid.UUID]uuid.UUID{
				artRead:   linkA,
				artLegacy: linkA,
			},
		}
		readingPort := &stubReadingSourcesWindowPort{
			entries: []domain.ReadingSourceEntry{
				{ArticleID: artRead, ReadRuleVersion: 1},
				{ArticleID: artLegacy, ReadRuleVersion: 0},
			},
		}
		h := NewHandler(FeedHandlerDeps{
			FeedInflowStats: feed_inflow_stats_usecase.NewFeedInflowStatsUsecase(inflowPort, readingPort),
			ResolveArticle:  resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		resp, err := h.GetFeedInflowStats(authCtxForUser(userID), connect.NewRequest(&feedsv2.GetFeedInflowStatsRequest{}))
		require.NoError(t, err)
		assert.Equal(t, int32(domain.FeedInflowWindowDays), resp.Msg.GetWindowDays())
		require.Len(t, resp.Msg.GetStats(), 1)
		st := resp.Msg.GetStats()[0]
		assert.Equal(t, linkA.String(), st.GetFeedLinkId())
		assert.Equal(t, int64(10), st.GetInflowCount())
		assert.Equal(t, int64(1), st.GetReadCount())
		assert.Equal(t, int64(1), st.GetOpenedCountLegacy())
		assert.InDelta(t, 0.1, st.GetReadRate(), 1e-9)
		require.NotNil(t, st.DailyCap)
		assert.Equal(t, int32(5), st.GetDailyCap())
	})

	t.Run("propagates error without silent fallback", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			FeedInflowStats: feed_inflow_stats_usecase.NewFeedInflowStatsUsecase(
				&stubFeedInflowReadPort{},
				&stubReadingSourcesWindowPort{err: errors.New("sovereign down")},
			),
			ResolveArticle: resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		_, err := h.GetFeedInflowStats(authCtxForUser(userID), connect.NewRequest(&feedsv2.GetFeedInflowStatsRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})

	t.Run("panics when usecase is unwired", func(t *testing.T) {
		h := NewHandler(FeedHandlerDeps{
			ResolveArticle: resolve_article_usecase.New(nil, nil),
		}, &config.Config{}, slog.Default())

		assert.Panics(t, func() {
			_, _ = h.GetFeedInflowStats(authCtxForUser(userID), connect.NewRequest(&feedsv2.GetFeedInflowStatsRequest{}))
		})
	})
}

type datasetUnreadPort struct {
	dataset []*domain.FeedItem
}

func (p *datasetUnreadPort) FetchUnreadFeedsListCursor(_ context.Context, cursor *time.Time, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
	return p.query(cursor, nil, limit), nil
}

func (p *datasetUnreadPort) FetchUnreadFeedsListCursorWithID(_ context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
	return p.query(cursor, cursorID, limit), nil
}

func (p *datasetUnreadPort) query(cursor *time.Time, cursorID *uuid.UUID, limit int) []*domain.FeedItem {
	var out []*domain.FeedItem
	for _, item := range p.dataset {
		switch {
		case cursor == nil:
			out = append(out, item)
		case cursorID == nil:
			if item.PublishedParsed.Before(*cursor) {
				out = append(out, item)
			}
		default:
			if item.PublishedParsed.Before(*cursor) || (item.PublishedParsed.Equal(*cursor) && item.FeedID.String() < cursorID.String()) {
				out = append(out, item)
			}
		}
		if len(out) == limit {
			break
		}
	}
	return out
}

func TestGetUnreadFeeds_DailyCapMultiPagePagination(t *testing.T) {
	logger.InitLogger()
	day := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	intPtr := func(v int) *int { return &v }

	port := &datasetUnreadPort{
		dataset: []*domain.FeedItem{
			{FeedID: uuid.New(), Title: "A5", Link: "https://a.example/5", PublishedParsed: day.Add(15 * time.Hour), DailyRank: intPtr(5), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "B2", Link: "https://b.example/2", PublishedParsed: day.Add(14 * time.Hour), DailyRank: intPtr(2), DailyCap: nil},
			{FeedID: uuid.New(), Title: "A4", Link: "https://a.example/4", PublishedParsed: day.Add(13 * time.Hour), DailyRank: intPtr(4), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "A3", Link: "https://a.example/3", PublishedParsed: day.Add(12 * time.Hour), DailyRank: intPtr(3), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "A2", Link: "https://a.example/2", PublishedParsed: day.Add(11 * time.Hour), DailyRank: intPtr(2), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "B1", Link: "https://b.example/1", PublishedParsed: day.Add(10 * time.Hour), DailyRank: intPtr(1), DailyCap: nil},
			{FeedID: uuid.New(), Title: "A1", Link: "https://a.example/1", PublishedParsed: day.Add(9 * time.Hour), DailyRank: intPtr(1), DailyCap: intPtr(2)},
		},
	}

	h := NewHandler(FeedHandlerDeps{
		CachedFeedList: cached_feed_list_usecase.NewCachedFeedListUsecase(nil, port, nil),
		ResolveArticle: resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())
	ctx := createAuthContext()

	var cursor *string
	var gotTitles []string
	for page := 0; page < 10; page++ {
		resp, err := h.GetUnreadFeeds(ctx, connect.NewRequest(&feedsv2.GetUnreadFeedsRequest{
			Limit:  2,
			Cursor: cursor,
		}))
		require.NoError(t, err)
		if resp.Msg.GetHasMore() {
			assert.Len(t, resp.Msg.GetData(), 2, "non-final page must not be short")
			require.NotNil(t, resp.Msg.NextCursor)
		}
		for _, item := range resp.Msg.GetData() {
			gotTitles = append(gotTitles, item.GetTitle())
		}
		if !resp.Msg.GetHasMore() {
			break
		}
		cursor = resp.Msg.NextCursor
	}

	assert.Equal(t, []string{"B2", "A2", "B1", "A1"}, gotTitles)
}
