// ABOUTME: PostgreSQL implementation of ArticleRepository interface
// ABOUTME: Handles CRUD operations for Inoreader articles with proper error handling

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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

// Create creates a new article in the database
func (r *PostgreSQLArticleRepository) Create(ctx context.Context, article *models.Article) error {
	query := `
		INSERT INTO inoreader_articles (
			id, inoreader_id, subscription_id, article_url, title, author,
			published_at, fetched_at, processed
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.db.ExecContext(ctx, query,
		article.ID,
		article.InoreaderID,
		article.SubscriptionID,
		article.ArticleURL,
		article.Title,
		article.Author,
		article.PublishedAt,
		article.FetchedAt,
		article.Processed,
	)

	if err != nil {
		r.logger.Error("Failed to create article",
			"inoreader_id", article.InoreaderID,
			"error", err)
		return fmt.Errorf("failed to create article: %w", err)
	}

	r.logger.Debug("Created article successfully",
		"inoreader_id", article.InoreaderID,
		"subscription_id", article.SubscriptionID)
	
	return nil
}

// CreateBatch creates multiple articles in a single transaction
func (r *PostgreSQLArticleRepository) CreateBatch(ctx context.Context, articles []*models.Article) (int, error) {
	if len(articles) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO inoreader_articles (
			id, inoreader_id, subscription_id, article_url, title, author,
			published_at, fetched_at, processed
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (inoreader_id) DO NOTHING`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	created := 0
	for _, article := range articles {
		result, err := stmt.ExecContext(ctx,
			article.ID,
			article.InoreaderID,
			article.SubscriptionID,
			article.ArticleURL,
			article.Title,
			article.Author,
			article.PublishedAt,
			article.FetchedAt,
			article.Processed,
		)
		if err != nil {
			r.logger.Warn("Failed to insert article in batch",
				"inoreader_id", article.InoreaderID,
				"error", err)
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			created++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("Batch article creation completed",
		"total_articles", len(articles),
		"created", created,
		"skipped", len(articles)-created)

	return created, nil
}

// FindByInoreaderID finds an article by its Inoreader ID
func (r *PostgreSQLArticleRepository) FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error) {
	query := `
		SELECT id, inoreader_id, subscription_id, article_url, title, author,
		       published_at, fetched_at, processed
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
		       published_at, fetched_at, processed
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
		       published_at, fetched_at, processed
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
		       published_at, fetched_at, processed
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
		       published_at, fetched_at, processed
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
		    published_at = $6, processed = $7
		WHERE inoreader_id = $1`

	result, err := r.db.ExecContext(ctx, query,
		article.InoreaderID,
		article.SubscriptionID,
		article.ArticleURL,
		article.Title,
		article.Author,
		article.PublishedAt,
		article.Processed,
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