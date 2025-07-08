package fetch_feed_tags_port

import (
	"alt/domain"
	"context"
	"time"
)

type FetchFeedTagsPort interface {
	FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
}
