package article_gateway

import (
	"alt/driver/alt_db"
	"alt/port/article_url_lookup_port"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArticleURLLookupGateway adapts the alt_db.ArticleRepository.LookupArticleURL
// driver method to the port interface used by track_home_action_usecase.
type ArticleURLLookupGateway struct {
	alt_db *alt_db.AltDBRepository
}

var _ article_url_lookup_port.ArticleURLLookupPort = (*ArticleURLLookupGateway)(nil)

// NewArticleURLLookupGateway constructs the gateway from a shared pgx pool.
func NewArticleURLLookupGateway(pool *pgxpool.Pool) *ArticleURLLookupGateway {
	return &ArticleURLLookupGateway{
		alt_db: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

// LookupArticleURL returns the canonical source URL for the given article ID
// scoped to userID. Returns ("", nil) when the article is not found within
// the user's tenant.
func (g *ArticleURLLookupGateway) LookupArticleURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error) {
	if g.alt_db == nil {
		return "", errors.New("database connection not available")
	}
	return g.alt_db.LookupArticleURL(ctx, articleID, userID)
}
