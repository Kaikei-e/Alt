package latest_article_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// FetchLatestArticlePort defines the interface for fetching the latest article for a feed.
type FetchLatestArticlePort interface {
	FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error)
}
