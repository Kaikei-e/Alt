package alt_db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LookupArticleURL returns the canonical source URL for an article scoped to
// the calling user. Returns ("", nil) when the article does not exist (so the
// caller can decide whether to log or fall back). pgx.ErrNoRows is mapped to
// the empty-string return path, not propagated as an error.
//
// Tenant scoped: the WHERE clause includes user_id to prevent cross-tenant
// URL disclosure (security audit High #1).
func (r *ArticleRepository) LookupArticleURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error) {
	if r == nil || r.pool == nil {
		return "", errors.New("database connection not available")
	}
	if articleID == "" {
		return "", nil
	}

	const query = `SELECT url FROM articles WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL LIMIT 1`

	var foundURL string
	err := r.pool.QueryRow(ctx, query, articleID, userID).Scan(&foundURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return foundURL, nil
}
