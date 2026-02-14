package article_summary_port

import (
	"alt/domain"
	"context"
	"net/url"
)

// FetchArticleSummaryPort defines the interface for fetching AI-generated article summaries.
type FetchArticleSummaryPort interface {
	FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error)
}
