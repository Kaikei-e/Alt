package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const fetchArticleByURLQuery = `
	SELECT id, title, content, url
	FROM articles
	WHERE url = $1 AND deleted_at IS NULL
`

// FetchArticleByURL retrieves article content from database by URL
func (r *AltDBRepository) FetchArticleByURL(ctx context.Context, articleURL string) (*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	cleanURL := strings.TrimSpace(articleURL)
	if cleanURL == "" {
		return nil, errors.New("article url cannot be empty")
	}

	var article domain.ArticleContent
	err := r.pool.QueryRow(ctx, fetchArticleByURLQuery, cleanURL).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfoContext(ctx, "article not found in database", "url", cleanURL)
			return nil, nil // Return nil without error to indicate not found
		}
		err = fmt.Errorf("fetch article by url: %w", err)
		logger.SafeErrorContext(ctx, "failed to fetch article", "url", cleanURL, "error", err)
		return nil, err
	}

	logger.SafeInfoContext(ctx, "article retrieved from database", "url", cleanURL, "article_id", article.ID)
	return &article, nil
}

const fetchArticleByIDQuery = `
	SELECT id, title, content, url
	FROM articles
	WHERE id = $1 AND deleted_at IS NULL
`

// FetchArticleByID retrieves article content from database by ID
func (r *AltDBRepository) FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	cleanID := strings.TrimSpace(articleID)
	if cleanID == "" {
		return nil, errors.New("article id cannot be empty")
	}

	var article domain.ArticleContent
	err := r.pool.QueryRow(ctx, fetchArticleByIDQuery, cleanID).Scan(
		&article.ID,
		&article.Title,
		&article.Content,
		&article.URL,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfoContext(ctx, "article not found in database", "id", cleanID)
			return nil, nil // Return nil without error to indicate not found
		}
		err = fmt.Errorf("fetch article by id: %w", err)
		logger.SafeErrorContext(ctx, "failed to fetch article", "id", cleanID, "error", err)
		return nil, err
	}

	logger.SafeInfoContext(ctx, "article retrieved from database", "id", cleanID)
	return &article, nil
}
