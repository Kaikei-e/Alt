package repository

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"pre-processor/driver"
	"pre-processor/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ArticleRepository implementation.
type articleRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewArticleRepository creates a new article repository.
func NewArticleRepository(db *pgxpool.Pool, logger *slog.Logger) ArticleRepository {
	return &articleRepository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new article.
func (r *articleRepository) Create(ctx context.Context, article *models.Article) error {
	r.logger.InfoContext(ctx, "creating article", "url", article.URL)

	// Check for nil database
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("failed to create article: database connection is nil")
	}

	feedID, err := driver.GetFeedID(ctx, r.db, article.URL)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to get feed ID", "error", err, "url", article.URL)
		return fmt.Errorf("failed to get feed ID: %w", err)
	}

	article.FeedID = feedID

	// Use existing driver function
	if err := driver.CreateArticle(ctx, r.db, article); err != nil {
		r.logger.ErrorContext(ctx, "failed to create article", "error", err, "url", article.URL)
		return fmt.Errorf("failed to create article: %w", err)
	}

	r.logger.InfoContext(ctx, "article created successfully", "url", article.URL)

	return nil
}

// CheckExists checks if articles exist for the given URLs.
func (r *articleRepository) CheckExists(ctx context.Context, urls []string) (bool, error) {
	r.logger.InfoContext(ctx, "checking if articles exist", "count", len(urls))

	// Check for nil database
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("failed to check article existence: database connection is nil")
	}

	// Convert strings to url.URL
	urlObjs := make([]url.URL, len(urls))

	for i, urlStr := range urls {
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to parse URL", "url", urlStr, "error", err)
			return false, fmt.Errorf("failed to parse URL %s: %w", urlStr, err)
		}

		urlObjs[i] = *parsedURL
	}

	// Use existing driver function
	exists, err := driver.CheckArticleExists(ctx, r.db, urlObjs)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to check article existence", "error", err)
		return false, fmt.Errorf("failed to check article existence: %w", err)
	}

	r.logger.InfoContext(ctx, "checked article existence", "exists", exists)

	return exists, nil
}

// FindForSummarization finds articles that need summarization.
func (r *articleRepository) FindForSummarization(ctx context.Context, cursor *Cursor, limit int) ([]*models.Article, *Cursor, error) {
	r.logger.InfoContext(ctx, "finding articles for summarization", "limit", limit)

	// Check for nil database
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, nil, fmt.Errorf("failed to find articles for summarization: database connection is nil")
	}

	var lastCreatedAt *time.Time

	var lastID string

	if cursor != nil {
		lastCreatedAt = cursor.LastCreatedAt
		lastID = cursor.LastID
	}

	// Use existing driver function
	articles, finalCreatedAt, finalID, err := driver.GetArticlesForSummarization(ctx, r.db, lastCreatedAt, lastID, limit)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to find articles for summarization", "error", err)
		return nil, nil, fmt.Errorf("failed to find articles for summarization: %w", err)
	}

	// Create new cursor
	newCursor := &Cursor{
		LastCreatedAt: finalCreatedAt,
		LastID:        finalID,
	}

	r.logger.InfoContext(ctx, "found articles for summarization", "count", len(articles))

	return articles, newCursor, nil
}

// HasUnsummarizedArticles checks if there are articles without summaries.
func (r *articleRepository) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	r.logger.InfoContext(ctx, "checking for unsummarized articles")

	// Check for nil database
	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("failed to check for unsummarized articles: database connection is nil")
	}

	// Use existing driver function
	hasUnsummarized, err := driver.HasUnsummarizedArticles(ctx, r.db)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to check for unsummarized articles", "error", err)
		return false, fmt.Errorf("failed to check for unsummarized articles: %w", err)
	}

	r.logger.InfoContext(ctx, "checked for unsummarized articles", "has_unsummarized", hasUnsummarized)

	return hasUnsummarized, nil
}

// FindByID finds an article by its ID.
func (r *articleRepository) FindByID(ctx context.Context, articleID string) (*models.Article, error) {
	r.logger.InfoContext(ctx, "finding article by ID", "article_id", articleID)

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, fmt.Errorf("failed to find article by ID: database connection is nil")
	}

	article, err := driver.GetArticleByID(ctx, r.db, articleID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to find article by ID", "error", err, "article_id", articleID)
		return nil, fmt.Errorf("failed to find article by ID: %w", err)
	}

	if article == nil {
		r.logger.WarnContext(ctx, "article not found", "article_id", articleID)
		return nil, nil
	}

	r.logger.InfoContext(ctx, "found article by ID", "article_id", articleID)
	return article, nil
}

// FetchInoreaderArticles fetches articles from Inoreader source.
func (r *articleRepository) FetchInoreaderArticles(ctx context.Context, since time.Time) ([]*models.Article, error) {
	r.logger.InfoContext(ctx, "fetching inoreader articles", "since", since)

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return nil, fmt.Errorf("failed to fetch inoreader articles: database connection is nil")
	}

	articles, err := driver.GetInoreaderArticles(ctx, r.db, since)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to fetch inoreader articles", "error", err)
		return nil, fmt.Errorf("failed to fetch inoreader articles: %w", err)
	}

	r.logger.InfoContext(ctx, "fetched inoreader articles", "count", len(articles))
	return articles, nil
}

// UpsertArticles batch upserts articles into the database.
func (r *articleRepository) UpsertArticles(ctx context.Context, articles []*models.Article) error {
	r.logger.InfoContext(ctx, "upserting articles", "count", len(articles))

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("failed to upsert articles: database connection is nil")
	}

	if len(articles) == 0 {
		return nil
	}

	err := driver.UpsertArticlesBatch(ctx, r.db, articles)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to upsert articles", "error", err)
		return fmt.Errorf("failed to upsert articles: %w", err)
	}

	r.logger.InfoContext(ctx, "articles upserted successfully", "count", len(articles))
	return nil
}
