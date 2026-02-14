package alt_db

import (
	"context"
	"errors"
	"time"
)

// CreateArticleParams holds parameters for creating an article via internal API.
type CreateArticleParams struct {
	Title       string
	URL         string
	Content     string
	FeedID      string
	UserID      string
	PublishedAt time.Time
}

// CreateArticleInternal creates a new article and returns its ID.
// This is used by the internal API (service-to-service), not the user-facing API.
func (r *AltDBRepository) CreateArticleInternal(ctx context.Context, params CreateArticleParams) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `
		INSERT INTO articles (title, content, url, feed_id, user_id, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url, user_id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id),
			published_at = EXCLUDED.published_at
		RETURNING id
	`

	var articleID string
	err := r.pool.QueryRow(ctx, query,
		params.Title,
		params.Content,
		params.URL,
		params.FeedID,
		params.UserID,
		params.PublishedAt,
	).Scan(&articleID)
	if err != nil {
		return "", err
	}

	return articleID, nil
}
