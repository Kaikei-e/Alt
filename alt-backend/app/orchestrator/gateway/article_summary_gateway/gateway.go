package article_summary_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/article_summary_port"
	"context"
	"net/url"
)

// Verify interface compliance at compile time.
var _ article_summary_port.FetchArticleSummaryPort = (*Gateway)(nil)

// SummaryStore is the generated-summary read alt-data-hub serves (capability
// catalog §2.H W3-H7).
type SummaryStore interface {
	FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error)
}

// Gateway implements FetchArticleSummaryPort.
type Gateway struct {
	store SummaryStore
}

// NewGateway creates a new article summary gateway.
func NewGateway(store SummaryStore) *Gateway {
	if store == nil {
		panic("article_summary_gateway: SummaryStore is required (see .claude/rules/di-wiring.md)")
	}
	return &Gateway{store: store}
}

// FetchFeedSummary retrieves the AI-generated summary for a given feed URL.
func (g *Gateway) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	return g.store.FetchFeedSummary(ctx, feedURL)
}
