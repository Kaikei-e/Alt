package fetch_feed_details_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_detail_port"
	"context"
	"net/url"
)

type FeedsSummaryUsecase struct {
	fetchFeedDetailsPort fetch_feed_detail_port.FetchFeedDetailsPort
}

func NewFeedsSummaryUsecase(fetchFeedDetailsPort fetch_feed_detail_port.FetchFeedDetailsPort) *FeedsSummaryUsecase {
	return &FeedsSummaryUsecase{fetchFeedDetailsPort: fetchFeedDetailsPort}
}

func (u *FeedsSummaryUsecase) Execute(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	summary, err := u.fetchFeedDetailsPort.FetchFeedDetails(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	return summary, nil
}
