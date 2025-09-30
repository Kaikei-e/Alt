package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
)

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

	if err := r.SaveArticle(ctx, cleanURL, cleanTitle, content); err != nil {
		logger.SafeError("failed to save article", "url", cleanURL, "error", err)
		return err
	}

	logger.SafeInfo("article content saved", "url", cleanURL)
	return nil
}
