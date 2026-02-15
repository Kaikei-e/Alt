package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/domain"
	"pre-processor/driver"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SummaryRepository implementation.
type summaryRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewSummaryRepository creates a new summary repository.
func NewSummaryRepository(db *pgxpool.Pool, logger *slog.Logger) SummaryRepository {
	return &summaryRepository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new article summary.
func (r *summaryRepository) Create(ctx context.Context, summary *domain.ArticleSummary) error {
	// Validate input
	if summary == nil {
		r.logger.ErrorContext(ctx, "summary cannot be nil")
		return fmt.Errorf("summary cannot be nil")
	}

	if summary.ArticleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return fmt.Errorf("article ID cannot be empty")
	}

	r.logger.InfoContext(ctx, "creating article summary", "article_id", summary.ArticleID)

	// Use existing driver function
	if err := driver.CreateArticleSummary(ctx, r.db, summary); err != nil {
		r.logger.ErrorContext(ctx, "failed to create article summary", "error", err, "article_id", summary.ArticleID)
		return fmt.Errorf("failed to create article summary: %w", err)
	}

	r.logger.InfoContext(ctx, "article summary created successfully", "article_id", summary.ArticleID)

	return nil
}

// FindArticlesWithSummaries finds articles with summaries for quality checking.
func (r *summaryRepository) FindArticlesWithSummaries(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	// Validate limit
	if limit <= 0 {
		r.logger.ErrorContext(ctx, "limit must be positive", "limit", limit)
		return nil, nil, fmt.Errorf("limit must be positive")
	}

	r.logger.InfoContext(ctx, "finding articles with summaries", "limit", limit)

	var lastCreatedAt *time.Time

	var lastID string

	if cursor != nil {
		lastCreatedAt = cursor.LastCreatedAt
		lastID = cursor.LastID
	}

	// Use existing driver function
	articlesWithSummaries, finalCreatedAt, finalID, err := driver.GetArticlesWithSummaries(ctx, r.db, lastCreatedAt, lastID, limit)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to find articles with summaries", "error", err)
		return nil, nil, fmt.Errorf("failed to find articles with summaries: %w", err)
	}

	// Convert driver.ArticleWithSummary to domain.ArticleWithSummary
	result := make([]*domain.ArticleWithSummary, len(articlesWithSummaries))
	for i, item := range articlesWithSummaries {
		result[i] = &domain.ArticleWithSummary{
			ArticleID:       item.ArticleID,
			ArticleContent:  item.Content,
			SummaryJapanese: item.SummaryJapanese,
			SummaryID:       item.SummaryID,
		}
	}

	// Create new cursor
	newCursor := &domain.Cursor{
		LastCreatedAt: finalCreatedAt,
		LastID:        finalID,
	}

	r.logger.InfoContext(ctx, "found articles with summaries", "count", len(result))

	return result, newCursor, nil
}

// Delete deletes an article summary by article ID.
func (r *summaryRepository) Delete(ctx context.Context, articleID string) error {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return fmt.Errorf("article ID cannot be empty")
	}

	r.logger.InfoContext(ctx, "deleting article summary", "article_id", articleID)

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return fmt.Errorf("failed to delete article summary: database connection is nil")
	}

	query := `DELETE FROM article_summaries WHERE article_id = $1`

	_, err := r.db.Exec(ctx, query, articleID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to delete article summary", "error", err, "article_id", articleID)
		return fmt.Errorf("failed to delete article summary: %w", err)
	}

	r.logger.InfoContext(ctx, "article summary deleted successfully", "article_id", articleID)

	return nil
}

// Exists checks if an article summary exists by article ID.
func (r *summaryRepository) Exists(ctx context.Context, articleID string) (bool, error) {
	if articleID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return false, fmt.Errorf("article ID cannot be empty")
	}

	r.logger.DebugContext(ctx, "checking if article summary exists", "article_id", articleID)

	if r.db == nil {
		r.logger.ErrorContext(ctx, "database connection is nil")
		return false, fmt.Errorf("failed to check if article summary exists: database connection is nil")
	}

	query := `SELECT EXISTS(SELECT 1 FROM article_summaries WHERE article_id = $1)`

	var exists bool

	err := r.db.QueryRow(ctx, query, articleID).Scan(&exists)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to check if article summary exists", "error", err, "article_id", articleID)
		return false, fmt.Errorf("failed to check if article summary exists: %w", err)
	}

	r.logger.DebugContext(ctx, "article summary existence check completed", "article_id", articleID, "exists", exists)

	return exists, nil
}
