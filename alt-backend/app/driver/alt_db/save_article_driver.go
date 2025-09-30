package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const upsertArticleQuery = `
	INSERT INTO articles (title, content, url)
	VALUES ($1, $2, $3)
	ON CONFLICT (url) DO UPDATE
	SET title = EXCLUDED.title,
		content = EXCLUDED.content
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

	var articleID uuid.UUID
	if err := r.pool.QueryRow(ctx, upsertArticleQuery, cleanTitle, content, cleanURL).Scan(&articleID); err != nil {
		err = fmt.Errorf("upsert article content: %w", err)
		logger.SafeError("failed to save article", "url", cleanURL, "error", err)
		return err
	}

	logger.SafeInfo("article content saved", "url", cleanURL, "article_id", articleID.String())
	return nil
}
