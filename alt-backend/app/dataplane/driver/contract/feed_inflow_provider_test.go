//go:build contract

package contract

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/orchestrator/connect/v2/feeds"
	"alt/orchestrator/usecase/feed_inflow_stats_usecase"
	"alt/orchestrator/usecase/resolve_article_usecase"
	"alt/orchestrator/usecase/subscription_usecase"
)

type pactFeedInflowStub struct{}

func (pactFeedInflowStub) ListSubscriptions(context.Context, uuid.UUID) ([]*domain.FeedSource, error) {
	return nil, nil
}
func (pactFeedInflowStub) Subscribe(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (pactFeedInflowStub) Unsubscribe(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (pactFeedInflowStub) SetSubscriptionDailyCap(context.Context, uuid.UUID, uuid.UUID, *int) error {
	return nil
}

func (pactFeedInflowStub) GetFeedInflowByUser(context.Context, uuid.UUID, time.Time, time.Time) ([]domain.FeedInflow, error) {
	capVal := 5
	return []domain.FeedInflow{{
		FeedLinkID:  uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		FeedURL:     "https://example.com/feed.xml",
		DailyCap:    &capVal,
		InflowCount: 10,
	}}, nil
}

func (pactFeedInflowStub) BatchGetFeedLinkIDsByArticleIDs(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	linkID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	return map[uuid.UUID]uuid.UUID{
		uuid.MustParse("22222222-2222-4222-8222-222222222222"): linkID,
		uuid.MustParse("33333333-3333-4333-8333-333333333333"): linkID,
	}, nil
}

func (pactFeedInflowStub) ListReadingSourcesInWindow(context.Context, uuid.UUID, time.Time, time.Time) ([]domain.ReadingSourceEntry, error) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	return []domain.ReadingSourceEntry{
		{
			ArticleID:       uuid.MustParse("22222222-2222-4222-8222-222222222222"),
			FirstReadAt:     now,
			ReadRuleVersion: 1,
		},
		{
			ArticleID:       uuid.MustParse("33333333-3333-4333-8333-333333333333"),
			FirstReadAt:     now,
			ReadRuleVersion: 0,
		},
	}, nil
}

func mountFeedServiceInflowProcedures(mux *http.ServeMux) {
	stub := pactFeedInflowStub{}
	handler := feeds.NewHandler(feeds.FeedHandlerDeps{
		SetSubscriptionDailyCap: subscription_usecase.NewSetSubscriptionDailyCapUsecase(stub),
		FeedInflowStats:         feed_inflow_stats_usecase.NewFeedInflowStatsUsecase(stub, stub),
		ResolveArticle:          resolve_article_usecase.New(nil, nil),
	}, &config.Config{}, slog.Default())

	_, connectHandler := feedsv2connect.NewFeedServiceHandler(handler)
	authedHandler := withPactUserContext(connectHandler)
	mux.Handle("/alt.feeds.v2.FeedService/SetSubscriptionDailyCap", authedHandler)
	mux.Handle("/alt.feeds.v2.FeedService/GetFeedInflowStats", authedHandler)
}
