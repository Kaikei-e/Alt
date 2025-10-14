package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const upsertArticleQuery = `
	INSERT INTO articles (title, content, url, user_id)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (url) DO UPDATE
	SET title = EXCLUDED.title,
		content = EXCLUDED.content,
		user_id = EXCLUDED.user_id
	RETURNING id
`

// SaveArticle stores or updates article content keyed by URL.
func (r *AltDBRepository) SaveArticle(ctx context.Context, url, title, content string) error {
	if r == nil || r.pool == nil {
		return errors.New("database connection not available")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		return errors.New("article url cannot be empty")
	}

	if strings.TrimSpace(content) == "" {
		return errors.New("article content cannot be empty")
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = cleanURL
	}

	// Validate that title is not a URL (this would indicate a bug)
	if strings.HasPrefix(cleanTitle, "http://") || strings.HasPrefix(cleanTitle, "https://") {
		logger.SafeWarn("article title appears to be a URL, this may indicate a bug", "url", cleanURL, "title", cleanTitle)
	}

	// Extract user_id from context
	userContext, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("user context required: %w", err)
	}

	var articleID uuid.UUID
	if err := r.pool.QueryRow(ctx, upsertArticleQuery, cleanTitle, content, cleanURL, userContext.UserID).Scan(&articleID); err != nil {
		err = fmt.Errorf("upsert article content: %w", err)
		logger.SafeError("failed to save article", "url", cleanURL, "error", err)
		return err
	}

	logger.SafeInfo("article content saved", "url", cleanURL, "article_id", articleID.String(), "user_id", userContext.UserID)
	return nil
}
