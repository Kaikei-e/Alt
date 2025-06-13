package fetch_feed_detail_port

import (
	"alt/domain"
	"context"
	"net/url"
)

type FetchFeedDetailsPort interface {
	FetchFeedDetails(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error)
}
	