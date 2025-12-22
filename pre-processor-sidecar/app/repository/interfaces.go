// ABOUTME: Repository layer common interfaces for clean architecture
// ABOUTME: Defines contracts for data access operations across different repositories

package repository

import (
	"context"
	"time"

	"pre-processor-sidecar/models"

	"github.com/google/uuid"
)

// ArticleRepository interface for article database operations
type ArticleRepository interface {
	// Create operations
	Create(ctx context.Context, article *models.Article) error
	CreateBatch(ctx context.Context, articles []*models.Article) (int, error)

	// Read operations
	FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.Article, error)
	GetUnprocessed(ctx context.Context, limit int) ([]*models.Article, error)
	GetBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID, limit int, offset int) ([]*models.Article, error)
	GetRecentArticles(ctx context.Context, since time.Time, limit int) ([]*models.Article, error)

	// Update operations
	Update(ctx context.Context, article *models.Article) error
	MarkAsProcessed(ctx context.Context, inoreaderID string) error
	MarkBatchAsProcessed(ctx context.Context, inoreaderIDs []string) error

	// Delete operations
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByInoreaderID(ctx context.Context, inoreaderID string) error
	DeleteOld(ctx context.Context, olderThan time.Time) (int, error)

	// Count operations
	CountTotal(ctx context.Context) (int, error)
	CountUnprocessed(ctx context.Context) (int, error)
	CountBySubscriptionID(ctx context.Context, subscriptionID uuid.UUID) (int, error)
}

// SyncStateRepository interface for continuation token management
type SyncStateRepository interface {
	// Create operations
	Create(ctx context.Context, syncState *models.SyncState) error

	// Read operations
	FindByStreamID(ctx context.Context, streamID string) (*models.SyncState, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.SyncState, error)
	GetAll(ctx context.Context) ([]*models.SyncState, error)
	GetStaleStates(ctx context.Context, olderThan time.Time) ([]*models.SyncState, error)
	GetOldestOne(ctx context.Context) (*models.SyncState, error)

	// Update operations
	Update(ctx context.Context, syncState *models.SyncState) error
	UpdateContinuationToken(ctx context.Context, streamID, token string) error

	// Delete operations
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByStreamID(ctx context.Context, streamID string) error
	DeleteStale(ctx context.Context, olderThan time.Time) (int, error)

	// Utility operations
	CleanupStale(ctx context.Context, retentionDays int) (int, error)
}

// Note: SubscriptionRepository interface is defined in subscription_repository.go
// to avoid circular dependencies and interface conflicts
