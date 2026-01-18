package fetch_recent_articles_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FetchRecentArticlesGateway implements the FetchRecentArticlesPort interface
type FetchRecentArticlesGateway struct {
	altDB *alt_db.AltDBRepository
}

// NewFetchRecentArticlesGateway creates a new gateway instance
func NewFetchRecentArticlesGateway(pool *pgxpool.Pool) *FetchRecentArticlesGateway {
	return &FetchRecentArticlesGateway{
		altDB: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

// FetchRecentArticles retrieves articles published since the given time
func (g *FetchRecentArticlesGateway) FetchRecentArticles(ctx context.Context, since time.Time, limit int) ([]*domain.Article, error) {
	if g.altDB == nil {
		return nil, errors.New("database connection not available")
	}

	articles, err := g.altDB.FetchRecentArticles(ctx, since, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching recent articles", "error", err, "since", since)
		return nil, errors.New("error fetching recent articles")
	}

	return articles, nil
}
