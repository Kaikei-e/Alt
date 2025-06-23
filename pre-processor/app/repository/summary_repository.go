package repository

import (
	"context"
	"fmt"
	"log/slog"
	"pre-processor/driver"
	"pre-processor/models"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SummaryRepository implementation
type summaryRepository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

// NewSummaryRepository creates a new summary repository
func NewSummaryRepository(db *pgxpool.Pool, logger *slog.Logger) SummaryRepository {
	return &summaryRepository{
		db:     db,
		logger: logger,
	}
}

// Create creates a new article summary
func (r *summaryRepository) Create(ctx context.Context, summary *models.ArticleSummary) error {
	r.logger.Info("creating article summary", "article_id", summary.ArticleID)

	// Use existing driver function
	if err := driver.CreateArticleSummary(ctx, r.db, summary); err != nil {
		r.logger.Error("failed to create article summary", "error", err, "article_id", summary.ArticleID)
		return fmt.Errorf("failed to create article summary: %w", err)
	}

	r.logger.Info("article summary created successfully", "article_id", summary.ArticleID)
	return nil
}

// FindArticlesWithSummaries finds articles with summaries for quality checking
func (r *summaryRepository) FindArticlesWithSummaries(ctx context.Context, cursor *Cursor, limit int) ([]*models.ArticleWithSummary, *Cursor, error) {
	r.logger.Info("finding articles with summaries", "limit", limit)

	var lastCreatedAt *time.Time
	var lastID string

	if cursor != nil {
		lastCreatedAt = cursor.LastCreatedAt
		lastID = cursor.LastID
	}

	// Use existing driver function
	articlesWithSummaries, finalCreatedAt, finalID, err := driver.GetArticlesWithSummaries(ctx, r.db, lastCreatedAt, lastID, limit)
	if err != nil {
		r.logger.Error("failed to find articles with summaries", "error", err)
		return nil, nil, fmt.Errorf("failed to find articles with summaries: %w", err)
	}

	// Convert driver.ArticleWithSummary to models.ArticleWithSummary
	result := make([]*models.ArticleWithSummary, len(articlesWithSummaries))
	for i, item := range articlesWithSummaries {
		result[i] = &models.ArticleWithSummary{
			ArticleID:       item.ArticleID,
			ArticleContent:  item.Content,
			SummaryJapanese: item.SummaryJapanese,
			SummaryID:       item.SummaryID,
		}
	}

	// Create new cursor
	newCursor := &Cursor{
		LastCreatedAt: finalCreatedAt,
		LastID:        finalID,
	}

	r.logger.Info("found articles with summaries", "count", len(result))
	return result, newCursor, nil
}

// Delete deletes an article summary
func (r *summaryRepository) Delete(ctx context.Context, summaryID string) error {
	r.logger.Info("deleting article summary", "summary_id", summaryID)

	// GREEN PHASE: Minimal implementation - we'll need to add this to driver later
	query := `DELETE FROM article_summaries WHERE id = $1`

	_, err := r.db.Exec(ctx, query, summaryID)
	if err != nil {
		r.logger.Error("failed to delete article summary", "error", err, "summary_id", summaryID)
		return fmt.Errorf("failed to delete article summary: %w", err)
	}

	r.logger.Info("article summary deleted successfully", "summary_id", summaryID)
	return nil
}

// Exists checks if an article summary exists
func (r *summaryRepository) Exists(ctx context.Context, summaryID string) (bool, error) {
	r.logger.Debug("checking if article summary exists", "summary_id", summaryID)

	query := `SELECT EXISTS(SELECT 1 FROM article_summaries WHERE id = $1)`

	var exists bool
	err := r.db.QueryRow(ctx, query, summaryID).Scan(&exists)
	if err != nil {
		r.logger.Error("failed to check if article summary exists", "error", err, "summary_id", summaryID)
		return false, fmt.Errorf("failed to check if article summary exists: %w", err)
	}

	r.logger.Debug("article summary existence check completed", "summary_id", summaryID, "exists", exists)
	return exists, nil
}
