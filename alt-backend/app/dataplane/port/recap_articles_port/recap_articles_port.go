package recap_articles_port

//go:generate mockgen -source=recap_articles_port.go -destination=../../mocks/mock_recap_articles_port.go -package=mocks

import (
	"alt/domain"
	"context"
)

// RecapArticlesPort exposes the persistence boundary for fetching recap-ready articles.
type RecapArticlesPort interface {
	FetchRecapArticles(ctx context.Context, query domain.RecapArticlesQuery) (*domain.RecapArticlesPage, error)
}
