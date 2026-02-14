package fetch_latest_article_usecase

import (
	"alt/domain"
	"alt/port/latest_article_port"
	"context"

	"github.com/google/uuid"
)

// FetchLatestArticleUsecase handles fetching the latest article for a feed.
type FetchLatestArticleUsecase struct {
	port latest_article_port.FetchLatestArticlePort
}

// NewFetchLatestArticleUsecase creates a new usecase instance.
func NewFetchLatestArticleUsecase(port latest_article_port.FetchLatestArticlePort) *FetchLatestArticleUsecase {
	return &FetchLatestArticleUsecase{port: port}
}

// Execute fetches the latest article for a given feed ID.
func (u *FetchLatestArticleUsecase) Execute(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	return u.port.FetchLatestArticleByFeedID(ctx, feedID)
}
