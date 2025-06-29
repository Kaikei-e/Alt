package feed_url_link_port

import (
	"alt/driver/models"
	"context"
)

type FeedURLLinkPort interface {
	GetFeedURLsByArticleIDs(ctx context.Context, articleIDs []string) ([]models.FeedAndArticle, error)
}
