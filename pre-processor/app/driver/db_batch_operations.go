package driver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pre-processor/models"
	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseInterface defines the interface for database operations
type DatabaseInterface interface {
	BeginTx(ctx context.Context, opts interface{}) (interface{}, error)
	Exec(ctx context.Context, query string, args ...interface{}) (interface{}, error)
}

// BatchInsertArticles inserts multiple articles in a single transaction for better performance
func BatchInsertArticles(ctx context.Context, db interface{}, articles []models.Article) error {
	if len(articles) == 0 {
		return nil
	}

	// Handle mock database for testing
	if mockDB, ok := db.(*MockDB); ok {
		return batchInsertMock(ctx, mockDB, articles)
	}

	// Handle real database connection
	pool, ok := db.(*pgxpool.Pool)
	if !ok {
		return fmt.Errorf("invalid database connection type")
	}

	if pool == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Build bulk insert query
	query := `
		INSERT INTO articles (
			title, content, url, feed_id, created_at
		) VALUES `

	values := make([]interface{}, 0, len(articles)*5)
	placeholders := make([]string, 0, len(articles))

	for i, article := range articles {
		placeholder := fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d)",
			i*5+1, i*5+2, i*5+3, i*5+4, i*5+5,
		)
		placeholders = append(placeholders, placeholder)

		now := time.Now()
		values = append(values,
			article.Title,
			article.Content,
			article.URL,
			article.FeedID,
			now,
		)
	}

	query += strings.Join(placeholders, ", ")
	query += ` ON CONFLICT (url) DO UPDATE SET
		title = EXCLUDED.title,
		content = EXCLUDED.content,
		feed_id = EXCLUDED.feed_id`

	// Execute batch insert
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to begin transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = tx.Exec(ctx, query, values...)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			logger.Logger.ErrorContext(ctx, "Failed to rollback transaction", "error", rollbackErr)
		}
		logger.Logger.ErrorContext(ctx, "Failed to batch insert articles", "error", err)
		return fmt.Errorf("failed to batch insert articles: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to commit transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Batch inserted articles", "count", len(articles))
	return nil
}

// BatchUpdateArticles updates multiple articles in a single transaction
func BatchUpdateArticles(ctx context.Context, db interface{}, articles []models.Article) error {
	if len(articles) == 0 {
		return nil
	}

	// Handle mock database for testing
	if mockDB, ok := db.(*MockDB); ok {
		return batchUpdateMock(ctx, mockDB, articles)
	}

	// Handle real database connection
	pool, ok := db.(*pgxpool.Pool)
	if !ok {
		return fmt.Errorf("invalid database connection type")
	}

	if pool == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Build batch update query using CASE WHEN
	query := `
		UPDATE articles SET
			title = CASE
	`

	var values []interface{}
	var ids []string

	for i, article := range articles {
		query += fmt.Sprintf(" WHEN id = $%d THEN $%d", i*3+1, i*3+2)
		values = append(values, article.ID, article.Title)
		ids = append(ids, article.ID)
	}

	query += " END, content = CASE"

	for i, article := range articles {
		query += fmt.Sprintf(" WHEN id = $%d THEN $%d", i*3+1, i*3+3)
		values = append(values, article.Content)
	}

	query += " END WHERE id IN ("
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", len(values)+i+1)
		values = append(values, id)
	}
	query += strings.Join(placeholders, ", ") + ")"

	// Execute batch update
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to begin transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = tx.Exec(ctx, query, values...)
	if err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			logger.Logger.ErrorContext(ctx, "Failed to rollback transaction", "error", rollbackErr)
		}
		logger.Logger.ErrorContext(ctx, "Failed to batch update articles", "error", err)
		return fmt.Errorf("failed to batch update articles: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to commit transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Batch updated articles", "count", len(articles))
	return nil
}

// Mock implementations for testing
func batchInsertMock(ctx context.Context, db *MockDB, articles []models.Article) error {
	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)
	return nil
}

func batchUpdateMock(ctx context.Context, db *MockDB, articles []models.Article) error {
	// Simulate some processing time
	time.Sleep(10 * time.Millisecond)
	return nil
}
