package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const fetchLatestArticleByFeedQuery = `
	SELECT id, title, content, url
	FROM articles
	WHERE feed_id = $1
	ORDER BY created_at DESC
	LIMIT 1
`

// FetchLatestArticleByFeedID retrieves the most recent article for a given feed.
// Returns nil (not an error) if no articles exist for the feed.
func (r *AltDBRepository) FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	var article domain.ArticleContent
	err := r.pool.QueryRow(ctx, fetchLatestArticleByFeedQuery, feedID).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Logger.InfoContext(ctx, "no articles found for feed", "feedID", feedID)
			return nil, nil // Return nil without error to indicate not found
		}
		logger.Logger.ErrorContext(ctx, "failed to fetch latest article by feed", "feedID", feedID, "error", err)
		return nil, errors.New("error fetching latest article for feed")
	}

	logger.Logger.InfoContext(ctx, "fetched latest article for feed", "feedID", feedID, "articleID", article.ID)
	return &article, nil
}
