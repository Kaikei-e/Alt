package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ArticleContent represents article data retrieved from database
type ArticleContent struct {
	ID      string
	Title   string
	Content string
	URL     string
}

const fetchArticleByURLQuery = `
	SELECT id, title, content, url
	FROM articles
	WHERE url = $1
`

// FetchArticleByURL retrieves article content from database by URL
func (r *AltDBRepository) FetchArticleByURL(ctx context.Context, articleURL string) (*ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	cleanURL := strings.TrimSpace(articleURL)
	if cleanURL == "" {
		return nil, errors.New("article url cannot be empty")
	}

	var article ArticleContent
	err := r.pool.QueryRow(ctx, fetchArticleByURLQuery, cleanURL).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfo("article not found in database", "url", cleanURL)
			return nil, nil // Return nil without error to indicate not found
		}
		err = fmt.Errorf("fetch article by url: %w", err)
		logger.SafeError("failed to fetch article", "url", cleanURL, "error", err)
		return nil, err
	}

	logger.SafeInfo("article retrieved from database", "url", cleanURL, "article_id", article.ID)
	return &article, nil
}
