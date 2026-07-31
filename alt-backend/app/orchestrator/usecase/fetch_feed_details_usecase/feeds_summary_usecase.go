package fetch_feed_details_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_detail_port"
	apperrors "alt/utils/errors"
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
	// (nil, nil) is the data-hub gateway's "no summary generated yet"
	// contract — an unset field over RPC, because the summarize path treats
	// that fact as "go generate one", not as a fault. At this endpoint the
	// caller asked to read a summary that does not exist, so surface it as a
	// typed not-found instead of letting the handler serialize a null 200.
	if summary == nil {
		return nil, apperrors.NewFeedNotFoundError(
			"usecase", "FeedsSummaryUsecase", "Execute",
			map[string]interface{}{"feed_url": feedURL.String()},
		)
	}
	return summary, nil
}
