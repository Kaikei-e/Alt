package article_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

type FetchArticlesGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFetchArticlesGateway(pool *pgxpool.Pool) *FetchArticlesGateway {
	return &FetchArticlesGateway{
		alt_db: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

// FetchArticlesWithCursor retrieves articles with tags using cursor-based pagination
func (g *FetchArticlesGateway) FetchArticlesWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "gateway.FetchArticlesWithCursor")
	defer span.End()

	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	articles, err := g.alt_db.FetchArticlesWithCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching articles with cursor", "error", err)
		return nil, errors.New("error fetching articles with cursor")
	}

	return articles, nil
}

func (g *FetchArticlesGateway) FetchArticleIDsWithCursor(ctx context.Context, cursor *time.Time, limit int) ([]uuid.UUID, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "gateway.FetchArticleIDsWithCursor")
	defer span.End()

	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	ids, err := g.alt_db.FetchArticleIDsWithCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching article ids with cursor", "error", err)
		return nil, errors.New("error fetching article ids with cursor")
	}

	return ids, nil
}
