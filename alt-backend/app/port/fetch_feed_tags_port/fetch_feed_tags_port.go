package fetch_feed_tags_port

import (
	"alt/domain"
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen -source=fetch_feed_tags_port.go -destination=../../mocks/mock_fetch_feed_tags_port.go

type FetchFeedTagsPort interface {
	FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
}