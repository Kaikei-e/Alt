package feed_url_link_port

import (
	"alt/domain"
	"context"
)

//go:generate go run go.uber.org/mock/mockgen -source=feed_url_link_port.go -destination=../../mocks/mock_feed_url_link_port.go -package=mocks FeedURLLinkPort

type FeedURLLinkPort interface {
	GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]domain.FeedAndArticle, error)
}
