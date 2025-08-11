//go:generate mockgen -source=subscription_repository.go -destination=../mocks/subscription_repository_mock.go -package=mocks SubscriptionRepository

// ABOUTME: This file handles subscription data persistence for Inoreader subscriptions
// ABOUTME: Implements database operations for storing and retrieving subscription data

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

// SubscriptionRepository interface for subscription data operations
type SubscriptionRepository interface {
	SaveSubscriptions(ctx context.Context, subscriptions []models.InoreaderSubscription) error
	GetAllSubscriptions(ctx context.Context) ([]models.InoreaderSubscription, error)
	// GetAll is an alias for GetAllSubscriptions for backward compatibility
	GetAll(ctx context.Context) ([]models.InoreaderSubscription, error)
	UpdateSubscription(ctx context.Context, subscription models.InoreaderSubscription) error
	DeleteSubscription(ctx context.Context, inoreaderID string) error
	// CreateSubscription creates a single subscription record for auto-creation functionality
	CreateSubscription(ctx context.Context, subscription *models.Subscription) error
}

// PostgreSQLSubscriptionRepository implements SubscriptionRepository using PostgreSQL
type PostgreSQLSubscriptionRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewPostgreSQLSubscriptionRepository creates a new PostgreSQL subscription repository
func NewPostgreSQLSubscriptionRepository(db *sql.DB, logger *slog.Logger) SubscriptionRepository {
	return &PostgreSQLSubscriptionRepository{
		db:     db,
		logger: logger,
	}
}

// SaveSubscriptions saves multiple subscriptions to database
func (r *PostgreSQLSubscriptionRepository) SaveSubscriptions(ctx context.Context, subscriptions []models.InoreaderSubscription) error {
	if len(subscriptions) == 0 {
		r.logger.Info("No subscriptions to save")
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare upsert statement (INSERT ... ON CONFLICT DO UPDATE)
	query := `
		INSERT INTO inoreader_subscriptions (
			id, inoreader_id, title, category, feed_url,
			synced_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (inoreader_id) DO UPDATE SET
			title = EXCLUDED.title,
			category = EXCLUDED.category,
			feed_url = EXCLUDED.feed_url,
			synced_at = EXCLUDED.synced_at
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	saved := 0

	for _, sub := range subscriptions {
		// Generate UUID for new subscriptions
		id := uuid.New().String()

		// Extract category from categories slice (use first category if multiple)
		category := ""
		if len(sub.Categories) > 0 {
			category = sub.Categories[0].Label
		}

		_, err := stmt.ExecContext(ctx,
			id,
			sub.InoreaderID, // Inoreader ID
			sub.Title,
			category, // Extracted from Categories
			sub.URL,  // Feed URL
			now,      // synced_at
			now,      // created_at
		)
		if err != nil {
			r.logger.Error("Failed to save subscription",
				"inoreader_id", sub.InoreaderID,
				"title", sub.Title,
				"error", err)
			continue
		}
		saved++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("Successfully saved subscriptions to database",
		"total_subscriptions", len(subscriptions),
		"saved_count", saved)

	return nil
}

// GetAllSubscriptions retrieves all subscriptions from database
func (r *PostgreSQLSubscriptionRepository) GetAllSubscriptions(ctx context.Context) ([]models.InoreaderSubscription, error) {
	query := `
		SELECT id, inoreader_id, title, category, feed_url,
		       synced_at, created_at
		FROM inoreader_subscriptions
		ORDER BY title
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer rows.Close()

	var subscriptions []models.InoreaderSubscription
	for rows.Next() {
		var sub models.InoreaderSubscription
		var createdAt, syncedAt time.Time
		var category string

		err := rows.Scan(
			&sub.DatabaseID,  // Database UUID - directly scan into UUID field
			&sub.InoreaderID, // Inoreader ID
			&sub.Title,
			&category,
			&sub.URL,
			&syncedAt,
			&createdAt,
		)
		if err != nil {
			r.logger.Error("Failed to scan subscription row", "error", err)
			continue
		}

		// Convert category string back to Categories slice
		if category != "" {
			sub.Categories = []models.InoreaderCategory{
				{Label: category},
			}
		}

		subscriptions = append(subscriptions, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return subscriptions, nil
}

// GetAll is an alias for GetAllSubscriptions (backward compatibility)
func (r *PostgreSQLSubscriptionRepository) GetAll(ctx context.Context) ([]models.InoreaderSubscription, error) {
	return r.GetAllSubscriptions(ctx)
}

// UpdateSubscription updates a single subscription
func (r *PostgreSQLSubscriptionRepository) UpdateSubscription(ctx context.Context, subscription models.InoreaderSubscription) error {
	// Extract category from categories slice (use first category if multiple)
	category := ""
	if len(subscription.Categories) > 0 {
		category = subscription.Categories[0].Label
	}

	query := `
		UPDATE inoreader_subscriptions
		SET title = $2, category = $3, feed_url = $4, synced_at = $5
		WHERE inoreader_id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		subscription.InoreaderID, // Inoreader ID
		subscription.Title,
		category,         // Extracted from Categories
		subscription.URL, // Feed URL
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// DeleteSubscription deletes a subscription by Inoreader ID
func (r *PostgreSQLSubscriptionRepository) DeleteSubscription(ctx context.Context, inoreaderID string) error {
	query := `DELETE FROM inoreader_subscriptions WHERE inoreader_id = $1`

	result, err := r.db.ExecContext(ctx, query, inoreaderID)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("subscription not found: %s", inoreaderID)
	}

	return nil
}

// CreateSubscription creates a single subscription record for auto-creation functionality
func (r *PostgreSQLSubscriptionRepository) CreateSubscription(ctx context.Context, subscription *models.Subscription) error {
	query := `
		INSERT INTO inoreader_subscriptions (
			id, inoreader_id, feed_url, title, category, synced_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (inoreader_id) DO UPDATE SET
			feed_url = EXCLUDED.feed_url,
			title = EXCLUDED.title,
			category = EXCLUDED.category,
			synced_at = EXCLUDED.synced_at`

	_, err := r.db.ExecContext(ctx, query,
		subscription.ID,
		subscription.InoreaderID,
		subscription.FeedURL,
		subscription.Title,
		subscription.Category,
		subscription.SyncedAt,
		subscription.CreatedAt,
	)

	if err != nil {
		r.logger.Error("Failed to create subscription",
			"inoreader_id", subscription.InoreaderID,
			"error", err)
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	r.logger.Info("Successfully created subscription",
		"inoreader_id", subscription.InoreaderID,
		"title", subscription.Title,
		"feed_url", subscription.FeedURL)

	return nil
}
