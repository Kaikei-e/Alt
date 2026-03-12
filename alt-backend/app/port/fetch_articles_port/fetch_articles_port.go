package fetch_articles_port

//go:generate go run go.uber.org/mock/mockgen -source=fetch_articles_port.go -destination=../../mocks/mock_fetch_articles_port.go -package=mocks FetchArticlesPort

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

type FetchArticlesPort interface {
	FetchArticlesWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error)
	FetchArticleIDsWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]uuid.UUID, error)
}
