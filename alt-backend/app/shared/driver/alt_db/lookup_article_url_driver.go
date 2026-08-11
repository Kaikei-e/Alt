package alt_db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"alt/domain"
)

// LookupArticleSource returns the canonical source URL and stored title for an
// article scoped to the calling user. Returns (zero, nil) when the article does
// not exist (so the caller can decide whether to log or fall back).
// pgx.ErrNoRows is mapped to the zero return path, not propagated as an error.
//
// Tenant scoped: the WHERE clause includes user_id to prevent cross-tenant
// URL disclosure (security audit High #1).
func (r *ArticleRepository) LookupArticleSource(ctx context.Context, articleID string, userID uuid.UUID) (domain.ArticleSource, error) {
	if r == nil || r.pool == nil {
		return domain.ArticleSource{}, errors.New("database connection not available")
	}
	if articleID == "" {
		return domain.ArticleSource{}, nil
	}

	const query = `SELECT url FROM articles WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL LIMIT 1`

	var found domain.ArticleSource
	err := r.pool.QueryRow(ctx, query, articleID, userID).Scan(&found.URL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ArticleSource{}, nil
		}
		return domain.ArticleSource{}, err
	}
	return found, nil
}
