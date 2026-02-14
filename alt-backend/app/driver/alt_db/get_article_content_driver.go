package alt_db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// InternalArticleContent represents article content for the internal API.
type InternalArticleContent struct {
	ID      string
	Title   string
	Content string
	URL     string
}

// GetArticleContent retrieves article content by ID for summarization.
func (r *AltDBRepository) GetArticleContent(ctx context.Context, articleID string) (*InternalArticleContent, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `SELECT id, title, content, url FROM articles WHERE id = $1 AND deleted_at IS NULL`

	var article InternalArticleContent
	err := r.pool.QueryRow(ctx, query, articleID).Scan(
		&article.ID, &article.Title, &article.Content, &article.URL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &article, nil
}
