// ABOUTME: PostgreSQL implementation of ArticleRepository interface
// ABOUTME: Handles CRUD operations for Inoreader articles with proper error handling

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pre-processor-sidecar/models"
	"github.com/google/uuid"
)

// PostgreSQLArticleRepository implements ArticleRepository using PostgreSQL
type PostgreSQLArticleRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewPostgreSQLArticleRepository creates a new PostgreSQL article repository
func NewPostgreSQLArticleRepository(db *sql.DB, logger *slog.Logger) ArticleRepository {
	return &PostgreSQLArticleRepository{
		db:     db,
		logger: logger,
	}
}

// UpsertResult represents the result of an upsert operation
type UpsertResult struct {
	WasInserted bool // true if new record was inserted, false if existing record was updated
}

// Create creates a new article in the database using UPSERT for idempotency
func (r *PostgreSQLArticleRepository) Create(ctx context.Context, article *models.Article) error {
	_, err := r.CreateWithResult(ctx, article)
	return err
}

// CreateWithResult creates an article and returns whether it was inserted or updated
func (r *PostgreSQLArticleRepository) CreateWithResult(ctx context.Context, article *models.Article) (*UpsertResult, error) {
	query := `
		INSERT INTO inoreader_articles (
			id, inoreader_id, subscription_id, article_url, title, author,
			published_at, fetched_at, processed, content, content_length, content_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (inoreader_id) 
		DO UPDATE SET
			subscription_id = EXCLUDED.subscription_id,
			article_url = EXCLUDED.article_url,
			title = EXCLUDED.title,
			author = EXCLUDED.author,
			published_at = EXCLUDED.published_at,
			fetched_at = EXCLUDED.fetched_at,
			content = EXCLUDED.content,
			content_length = EXCLUDED.content_length,
			content_type = EXCLUDED.content_type
		RETURNING (xmax = 0) AS was_inserted`

	var wasInserted bool
	err := r.db.QueryRowContext(ctx, query,
		article.ID,
		article.InoreaderID,
		article.SubscriptionID,
		article.ArticleURL,
		article.Title,
		article.Author,
		article.PublishedAt,
		article.FetchedAt,
		article.Processed,
		article.Content,
		article.ContentLength,
		article.ContentType,
	).Scan(&wasInserted)

	if err != nil {
		r.logger.Error("Failed to upsert article",
			"inoreader_id", article.InoreaderID,
			"error", err)
		return nil, fmt.Errorf("failed to upsert article: %w", err)
	}

	if wasInserted {
		r.logger.Debug("Created new article",
			"inoreader_id", article.InoreaderID,
			"subscription_id", article.SubscriptionID)
	} else {
		r.logger.Debug("Updated existing article",
			"inoreader_id", article.InoreaderID,
			"subscription_id", article.SubscriptionID)
	}
	
	return &UpsertResult{WasInserted: wasInserted}, nil
}

// BatchResult contains statistics about batch operation
type BatchResult struct {
	Total     int
	Inserted  int
	Updated   int
	Failed    int
	LastError error
}

// CreateBatch creates multiple articles using UPSERT for efficient duplicate handling
func (r *PostgreSQLArticleRepository) CreateBatch(ctx context.Context, articles []*models.Article) (int, error) {
	result := r.CreateBatchWithResult(ctx, articles)
	return result.Inserted + result.Updated, result.LastError
}

// CreateBatchWithResult creates multiple articles and returns detailed statistics
func (r *PostgreSQLArticleRepository) CreateBatchWithResult(ctx context.Context, articles []*models.Article) *BatchResult {
	result := &BatchResult{Total: len(articles)}

	if len(articles) == 0 {
		return result
	}

	r.logger.Info("Starting resilient batch article creation",
		"total_articles", len(articles))

	// Process each article using UPSERT with detailed tracking
	for i, article := range articles {
		upsertResult, err := r.createArticleWithValidationAndResult(ctx, article)
		if err != nil {
			// Check if it's a foreign key violation (subscription missing)
			if r.isForeignKeyError(err) {
				r.logger.Warn("Foreign key violation - subscription missing",
					"inoreader_id", article.InoreaderID,
					"subscription_id", article.SubscriptionID,
					"error", err)
			} else {
				// Other errors (should be rare with UPSERT)
				r.logger.Error("Failed to upsert article",
					"inoreader_id", article.InoreaderID,
					"article_index", i+1,
					"error", err)
			}
			result.Failed++
			result.LastError = err
			continue
		}

		if upsertResult.WasInserted {
			result.Inserted++
		} else {
			result.Updated++
		}

		r.logger.Debug("Article upserted successfully",
			"inoreader_id", article.InoreaderID,
			"subscription_id", article.SubscriptionID,
			"was_new", upsertResult.WasInserted,
			"progress", fmt.Sprintf("%d/%d", i+1, len(articles)))
	}

	successCount := result.Inserted + result.Updated
	r.logger.Info("Resilient batch article creation completed",
		"total_articles", result.Total,
		"inserted", result.Inserted,
		"updated", result.Updated,
		"failed", result.Failed,
		"success_rate", fmt.Sprintf("%.1f%%", float64(successCount)/float64(result.Total)*100),
		"new_vs_duplicate_ratio", fmt.Sprintf("%d:%d", result.Inserted, result.Updated))

	// Set error only if all articles failed
	if successCount == 0 && result.Failed > 0 && result.LastError != nil {
		result.LastError = fmt.Errorf("all articles failed to upsert, last error: %w", result.LastError)
	} else {
		result.LastError = nil
	}

	return result
}

// createArticleWithValidation creates a single article with pre-validation
func (r *PostgreSQLArticleRepository) createArticleWithValidation(ctx context.Context, article *models.Article) error {
	_, err := r.createArticleWithValidationAndResult(ctx, article)
	return err
}

// createArticleWithValidationAndResult creates a single article with pre-validation and returns result
func (r *PostgreSQLArticleRepository) createArticleWithValidationAndResult(ctx context.Context, article *models.Article) (*UpsertResult, error) {
	// Pre-validation: Check for nil UUID (invalid subscription)
	if article.SubscriptionID == uuid.Nil {
		return nil, fmt.Errorf("invalid subscription ID: nil UUID for inoreader_id %s", article.InoreaderID)
	}

	// Validate required fields
	if article.InoreaderID == "" {
		return nil, fmt.Errorf("invalid article: empty inoreader_id")
	}
	if article.ArticleURL == "" {
		return nil, fmt.Errorf("invalid article: empty article_url for inoreader_id %s", article.InoreaderID)
	}

	// Create article using UPSERT with result tracking
	return r.CreateWithResult(ctx, article)
}

// isDuplicateError is deprecated - no longer needed with UPSERT implementation
// Left for compatibility with existing code that might call it
func (r *PostgreSQLArticleRepository) isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errStr := fmt.Sprintf("%v", err)
	// Check for PostgreSQL unique constraint violation (error code 23505)
	return strings.Contains(errStr, "duplicate key value violates unique constraint") ||
		strings.Contains(errStr, "inoreader_articles_inoreader_id_key")
}

// isForeignKeyError checks if error is due to foreign key constraint violation  
func (r *PostgreSQLArticleRepository) isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := fmt.Sprintf("%v", err)
	// Check for PostgreSQL foreign key constraint violation (error code 23503)
	return strings.Contains(errStr, "violates foreign key constraint") ||
		strings.Contains(errStr, "inoreader_articles_subscription_id_fkey")
}

// FindByInoreaderID finds an article by its Inoreader ID
func (r *PostgreSQLArticleRepository) FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed, content, content_length, content_type
		FROM inoreader_articles
		WHERE inoreader_id = $1`

	var article models.Article
	err := r.db.QueryRowContext(ctx, query, inoreaderID).Scan(
		&article.ID,
		&article.InoreaderID,
		&article.SubscriptionID,
		&article.ArticleURL,
		&article.Title,
		&article.Author,
		&article.PublishedAt,
		&article.FetchedAt,
		&article.Processed,
		&article.Content,
		&article.ContentLength,
		&article.ContentType,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("article not found with inoreader_id: %s", inoreaderID)
		}
		return nil, fmt.Errorf("failed to find article by inoreader_id: %w", err)
	}

	return &article, nil
}

// FindByID finds an article by its UUID
func (r *PostgreSQLArticleRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed, content, content_length, content_type
		FROM inoreader_articles
		WHERE id = $1`

	var article models.Article
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&article.ID,
		&article.InoreaderID,
		&article.SubscriptionID,
		&article.ArticleURL,
		&article.Title,
		&article.Author,
		&article.PublishedAt,
		&article.FetchedAt,
		&article.Processed,
		&article.Content,
		&article.ContentLength,
		&article.ContentType,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("article not found with id: %s", id.String())
		}
		return nil, fmt.Errorf("failed to find article by id: %w", err)
	}

	return &article, nil
}

// GetUnprocessed retrieves unprocessed articles with limit
func (r *PostgreSQLArticleRepository) GetUnprocessed(ctx context.Context, limit int) ([]*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed, content, content_length, content_type
		FROM inoreader_articles
		WHERE processed = false
		ORDER BY fetched_at ASC
		LIMIT $1`

	return r.queryArticles(ctx, query, limit)
}

// GetBySubscriptionID retrieves articles by subscription ID with pagination
func (r *PostgreSQLArticleRepository) GetBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID, limit int, offset int) ([]*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed, content, content_length, content_type
		FROM inoreader_articles
		WHERE subscription_id = $1
		ORDER BY published_at DESC NULLS LAST, fetched_at DESC
		LIMIT $2 OFFSET $3`

	return r.queryArticles(ctx, query, subscriptionID, limit, offset)
}

// GetRecentArticles retrieves recent articles since specified time
func (r *PostgreSQLArticleRepository) GetRecentArticles(ctx context.Context, since time.Time, limit int) ([]*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed, content, content_length, content_type
		FROM inoreader_articles
		WHERE fetched_at >= $1
		ORDER BY fetched_at DESC
		LIMIT $2`

	return r.queryArticles(ctx, query, since, limit)
}

// Update updates an existing article
func (r *PostgreSQLArticleRepository) Update(ctx context.Context, article *models.Article) error {
	query := `
		UPDATE inoreader_articles
		SET subscription_id = $2, article_url = $3, title = $4, author = $5,
		    published_at = $6, processed = $7, content = $8, content_length = $9, content_type = $10
		WHERE inoreader_id = $1`

	result, err := r.db.ExecContext(ctx, query,
		article.InoreaderID,
		article.SubscriptionID,
		article.ArticleURL,
		article.Title,
		article.Author,
		article.PublishedAt,
		article.Processed,
		article.Content,
		article.ContentLength,
		article.ContentType,
	)

	if err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("article not found for update: %s", article.InoreaderID)
	}

	return nil
}

// MarkAsProcessed marks an article as processed
func (r *PostgreSQLArticleRepository) MarkAsProcessed(ctx context.Context, inoreaderID string) error {
	query := `UPDATE inoreader_articles SET processed = true WHERE inoreader_id = $1`
	
	result, err := r.db.ExecContext(ctx, query, inoreaderID)
	if err != nil {
		return fmt.Errorf("failed to mark article as processed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("article not found for processing: %s", inoreaderID)
	}

	return nil
}

// MarkBatchAsProcessed marks multiple articles as processed
func (r *PostgreSQLArticleRepository) MarkBatchAsProcessed(ctx context.Context, inoreaderIDs []string) error {
	if len(inoreaderIDs) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `UPDATE inoreader_articles SET processed = true WHERE inoreader_id = $1`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	processed := 0
	for _, inoreaderID := range inoreaderIDs {
		result, err := stmt.ExecContext(ctx, inoreaderID)
		if err != nil {
			r.logger.Warn("Failed to mark article as processed in batch",
				"inoreader_id", inoreaderID,
				"error", err)
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			processed++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("Batch processed articles updated",
		"total_ids", len(inoreaderIDs),
		"processed", processed)

	return nil
}

// Delete deletes an article by ID
func (r *PostgreSQLArticleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM inoreader_articles WHERE id = $1`
	
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete article: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("article not found for deletion: %s", id.String())
	}

	return nil
}

// DeleteByInoreaderID deletes an article by Inoreader ID
func (r *PostgreSQLArticleRepository) DeleteByInoreaderID(ctx context.Context, inoreaderID string) error {
	query := `DELETE FROM inoreader_articles WHERE inoreader_id = $1`
	
	result, err := r.db.ExecContext(ctx, query, inoreaderID)
	if err != nil {
		return fmt.Errorf("failed to delete article by inoreader_id: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("article not found for deletion: %s", inoreaderID)
	}

	return nil
}

// DeleteOld deletes articles older than specified time
func (r *PostgreSQLArticleRepository) DeleteOld(ctx context.Context, olderThan time.Time) (int, error) {
	query := `DELETE FROM inoreader_articles WHERE fetched_at < $1`
	
	result, err := r.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old articles: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	deletedCount := int(rowsAffected)
	r.logger.Info("Deleted old articles",
		"count", deletedCount,
		"older_than", olderThan)

	return deletedCount, nil
}

// CountTotal returns the total number of articles
func (r *PostgreSQLArticleRepository) CountTotal(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM inoreader_articles`
	
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count total articles: %w", err)
	}

	return count, nil
}

// CountUnprocessed returns the number of unprocessed articles
func (r *PostgreSQLArticleRepository) CountUnprocessed(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM inoreader_articles WHERE processed = false`
	
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count unprocessed articles: %w", err)
	}

	return count, nil
}

// CountBySubscriptionID returns the number of articles for a subscription
func (r *PostgreSQLArticleRepository) CountBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM inoreader_articles WHERE subscription_id = $1`
	
	var count int
	err := r.db.QueryRowContext(ctx, query, subscriptionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count articles by subscription_id: %w", err)
	}

	return count, nil
}

// queryArticles is a helper method to execute queries that return multiple articles
func (r *PostgreSQLArticleRepository) queryArticles(ctx context.Context, query string, args ...interface{}) ([]*models.Article, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query articles: %w", err)
	}
	defer rows.Close()

	var articles []*models.Article
	for rows.Next() {
		article := &models.Article{}
		err := rows.Scan(
			&article.ID,
			&article.InoreaderID,
			&article.SubscriptionID,
			&article.ArticleURL,
			&article.Title,
			&article.Author,
			&article.PublishedAt,
			&article.FetchedAt,
			&article.Processed,
			&article.Content,
			&article.ContentLength,
			&article.ContentType,
		)
		if err != nil {
			r.logger.Error("Failed to scan article row", "error", err)
			continue
		}

		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return articles, nil
}