package fetch_feed_detail_gateway

import (
	"alt/domain"
	"context"
	"net/url"
)

// FeedSummaryStore is the generated-summary read (capability catalog §2.H
// W3-H7).
//
// It answers (nil, nil) for an article with no summary yet. That used to be
// pgx.ErrNoRows, which the caller read as a cache miss; over an RPC an error
// means a fault, so the absence became an unset field.
type FeedSummaryStore interface {
	FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error)
}

type FeedSummaryGateway struct {
	store FeedSummaryStore
}

func NewFeedSummaryGateway(store FeedSummaryStore) *FeedSummaryGateway {
	if store == nil {
		panic("fetch_feed_detail_gateway: FeedSummaryStore is required (see .claude/rules/di-wiring.md)")
	}
	return &FeedSummaryGateway{store: store}
}

func (g *FeedSummaryGateway) FetchFeedDetails(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	return g.store.FetchFeedSummary(ctx, feedURL)
}
