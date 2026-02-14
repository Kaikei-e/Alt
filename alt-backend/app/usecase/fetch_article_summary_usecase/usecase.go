package fetch_article_summary_usecase

import (
	"alt/domain"
	"alt/port/article_summary_port"
	"context"
	"net/url"
)

// FetchArticleSummaryUsecase handles fetching AI-generated article summaries.
type FetchArticleSummaryUsecase struct {
	port article_summary_port.FetchArticleSummaryPort
}

// NewFetchArticleSummaryUsecase creates a new usecase instance.
func NewFetchArticleSummaryUsecase(port article_summary_port.FetchArticleSummaryPort) *FetchArticleSummaryUsecase {
	return &FetchArticleSummaryUsecase{port: port}
}

// Execute fetches the AI-generated summary for a given feed URL.
func (u *FetchArticleSummaryUsecase) Execute(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	return u.port.FetchFeedSummary(ctx, feedURL)
}
