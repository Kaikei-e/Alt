package repository

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"pre-processor/driver"
	"pre-processor/models"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ArticleRepository implementation
type articleRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewArticleRepository creates a new article repository
func NewArticleRepository(db *pgxpool.Pool, logger *slog.Logger) ArticleRepository {
	return &articleRepository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new article
func (r *articleRepository) Create(ctx context.Context, article *models.Article) error {
	r.logger.Info("creating article", "url", article.URL)

	// Use existing driver function
	if err := driver.CreateArticle(ctx, r.db, article); err != nil {
		r.logger.Error("failed to create article", "error", err, "url", article.URL)
		return fmt.Errorf("failed to create article: %w", err)
	}

	r.logger.Info("article created successfully", "url", article.URL)
	return nil
}

// CheckExists checks if articles exist for the given URLs
func (r *articleRepository) CheckExists(ctx context.Context, urls []string) (bool, error) {
	r.logger.Info("checking if articles exist", "count", len(urls))

	// Convert strings to url.URL
	urlObjs := make([]url.URL, len(urls))
	for i, urlStr := range urls {
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			r.logger.Error("failed to parse URL", "url", urlStr, "error", err)
			return false, fmt.Errorf("failed to parse URL %s: %w", urlStr, err)
		}
		urlObjs[i] = *parsedURL
	}

	// Use existing driver function
	exists, err := driver.CheckArticleExists(ctx, r.db, urlObjs)
	if err != nil {
		r.logger.Error("failed to check article existence", "error", err)
		return false, fmt.Errorf("failed to check article existence: %w", err)
	}

	r.logger.Info("checked article existence", "exists", exists)
	return exists, nil
}

// FindForSummarization finds articles that need summarization
func (r *articleRepository) FindForSummarization(ctx context.Context, cursor *Cursor, limit int) ([]*models.Article, *Cursor, error) {
	r.logger.Info("finding articles for summarization", "limit", limit)

	var lastCreatedAt *time.Time
	var lastID string

	if cursor != nil {
		lastCreatedAt = cursor.LastCreatedAt
		lastID = cursor.LastID
	}

	// Use existing driver function
	articles, finalCreatedAt, finalID, err := driver.GetArticlesForSummarization(ctx, r.db, lastCreatedAt, lastID, limit)
	if err != nil {
		r.logger.Error("failed to find articles for summarization", "error", err)
		return nil, nil, fmt.Errorf("failed to find articles for summarization: %w", err)
	}

	// Create new cursor
	newCursor := &Cursor{
		LastCreatedAt: finalCreatedAt,
		LastID:        finalID,
	}

	r.logger.Info("found articles for summarization", "count", len(articles))
	return articles, newCursor, nil
}

// HasUnsummarizedArticles checks if there are articles without summaries
func (r *articleRepository) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	r.logger.Info("checking for unsummarized articles")

	// Use existing driver function
	hasUnsummarized, err := driver.HasUnsummarizedArticles(ctx, r.db)
	if err != nil {
		r.logger.Error("failed to check for unsummarized articles", "error", err)
		return false, fmt.Errorf("failed to check for unsummarized articles: %w", err)
	}

	r.logger.Info("checked for unsummarized articles", "has_unsummarized", hasUnsummarized)
	return hasUnsummarized, nil
}
