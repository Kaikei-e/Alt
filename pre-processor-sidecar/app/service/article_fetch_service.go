//go:generate mockgen -source=article_fetch_service.go -destination=../mocks/article_fetch_repositories_mock.go -package=mocks ArticleRepository,SyncStateRepository

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"

	"github.com/google/uuid"
)

// ArticleRepository interface for article database operations
type ArticleRepository interface {
	FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error)
	Create(ctx context.Context, article *models.Article) error
	Update(ctx context.Context, article *models.Article) error
	GetUnprocessed(ctx context.Context, limit int) ([]*models.Article, error)
	MarkAsProcessed(ctx context.Context, articleID string) error
	DeleteOld(ctx context.Context, olderThan time.Time) (int, error)
}

// SyncStateRepository interface for sync state operations
type SyncStateRepository interface {
	FindByStreamID(ctx context.Context, streamID string) (*models.SyncState, error)
	Create(ctx context.Context, syncState *models.SyncState) error
	Update(ctx context.Context, syncState *models.SyncState) error
}

// ArticleFetchResult represents the result of an article fetch operation
type ArticleFetchResult struct {
	NewArticles       int           `json:"new_articles"`
	TotalProcessed    int           `json:"total_processed"`
	ContinuationToken string        `json:"continuation_token,omitempty"`
	SyncTime          time.Time     `json:"sync_time"`
	Duration          time.Duration `json:"duration"`
	Errors            []string      `json:"errors,omitempty"`
}

// SubscriptionMapping represents the cache for mapping Inoreader stream IDs to subscription UUIDs
type SubscriptionMapping struct {
	InoreaderIDToUUID map[string]uuid.UUID // "feed/http://example.com/rss" -> UUID
	UUIDToInoreaderID map[uuid.UUID]string // UUID -> "feed/http://example.com/rss"
	LoadedAt          time.Time            // Cache creation timestamp
	TotalCount        int                  // Number of subscriptions loaded
}

// ArticleFetchService handles fetching articles from Inoreader API with continuation tokens
type ArticleFetchService struct {
	inoreaderService *InoreaderService
	articleRepo      ArticleRepository
	syncStateRepo    SyncStateRepository
	subscriptionRepo repository.SubscriptionRepository // Added for UUID resolution
	logger           *slog.Logger
	mu               sync.RWMutex
}

// (removed SubscriptionQueryRepository; use repository.SubscriptionRepository directly)

// NewArticleFetchService creates a new article fetch service
func NewArticleFetchService(
	inoreaderService *InoreaderService,
	articleRepo ArticleRepository,
	syncStateRepo SyncStateRepository,
	subscriptionRepo repository.SubscriptionRepository,
	logger *slog.Logger,
) *ArticleFetchService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	return &ArticleFetchService{
		inoreaderService: inoreaderService,
		articleRepo:      articleRepo,
		syncStateRepo:    syncStateRepo,
		subscriptionRepo: subscriptionRepo,
		logger:           logger,
	}
}

// FetchArticles fetches articles from a specific stream with continuation token support
func (s *ArticleFetchService) FetchArticles(ctx context.Context, streamID string, maxArticles int) (*ArticleFetchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	startTime := time.Now()
	s.logger.Info("Starting article fetch",
		"stream_id", streamID,
		"max_articles", maxArticles)

	result := &ArticleFetchResult{
		SyncTime: startTime,
		Errors:   []string{},
	}

	// Step 1: Build subscription mapping cache for UUID resolution
	subscriptionMapping, err := s.buildSubscriptionMapping(ctx)
	if err != nil {
		s.logger.Error("Failed to build subscription mapping", "error", err)
		return nil, fmt.Errorf("failed to build subscription mapping: %w", err)
	}

	// Step 2: Get existing sync state for continuation token
	syncState, err := s.syncStateRepo.FindByStreamID(ctx, streamID)
	if err != nil {
		s.logger.Debug("No existing sync state found, starting fresh fetch", "stream_id", streamID)
		syncState = nil
	}

	var continuationToken string
	if syncState != nil {
		continuationToken = syncState.ContinuationToken
		s.logger.Debug("Using existing continuation token",
			"stream_id", streamID,
			"continuation_token", continuationToken)
	}

	// Step 3: Fetch articles from Inoreader API
	articles, nextToken, err := s.inoreaderService.FetchStreamContents(ctx, streamID, continuationToken)
	if err != nil {
		s.logger.Error("Failed to fetch articles from Inoreader API", "error", err, "stream_id", streamID)
		return nil, fmt.Errorf("failed to fetch articles from stream %s: %w", streamID, err)
	}

	s.logger.Info("Fetched articles from Inoreader API",
		"stream_id", streamID,
		"count", len(articles),
		"next_token", nextToken)

	// Step 4: Resolve subscription UUIDs for all articles using cache
	unknownSubscriptions := 0
	for _, article := range articles {
		if subscriptionUUID, exists := subscriptionMapping.InoreaderIDToUUID[article.OriginStreamID]; exists {
			article.SubscriptionID = subscriptionUUID
		} else {
			unknownSubscriptions++
			s.logger.Warn("Unknown subscription for article",
				"article_inoreader_id", article.InoreaderID,
				"origin_stream_id", article.OriginStreamID)
			// Set to nil UUID - article will be skipped or handled differently
			article.SubscriptionID = uuid.Nil
		}
		// Clear the temporary field
		article.OriginStreamID = ""
	}

	if unknownSubscriptions > 0 {
		s.logger.Warn("Articles with unknown subscriptions found",
			"unknown_count", unknownSubscriptions,
			"total_articles", len(articles))
	}

	// Step 5: Process articles in batches
	processed, skipped, err := s.ProcessArticleBatch(ctx, articles)
	if err != nil {
		s.logger.Error("Failed to process article batch", "error", err)
		return nil, fmt.Errorf("failed to process article batch: %w", err)
	}

	result.NewArticles = processed
	result.TotalProcessed = len(articles)
	result.ContinuationToken = nextToken

	// Update or create sync state with new continuation token
	if err := s.updateSyncState(ctx, streamID, nextToken, syncState); err != nil {
		s.logger.Error("Failed to update sync state", "error", err, "stream_id", streamID)
		errorMsg := fmt.Sprintf("Failed to update sync state: %v", err)
		result.Errors = append(result.Errors, errorMsg)
	}

	result.Duration = time.Since(startTime)

	s.logger.Info("Article fetch completed",
		"stream_id", streamID,
		"duration", result.Duration,
		"new_articles", result.NewArticles,
		"total_processed", result.TotalProcessed,
		"skipped", skipped,
		"continuation_token", result.ContinuationToken)

	return result, nil
}

// buildSubscriptionMapping builds a cache mapping Inoreader stream IDs to subscription UUIDs
func (s *ArticleFetchService) buildSubscriptionMapping(ctx context.Context) (*SubscriptionMapping, error) {
	s.logger.Debug("Building subscription mapping cache")

	startTime := time.Now()

	// Fetch all subscriptions from database in a single query
	subscriptions, err := s.subscriptionRepo.GetAllSubscriptions(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch all subscriptions", "error", err)
		return nil, fmt.Errorf("failed to fetch subscriptions for mapping: %w", err)
	}

	// Build bidirectional mapping
	mapping := &SubscriptionMapping{
		InoreaderIDToUUID: make(map[string]uuid.UUID, len(subscriptions)),
		UUIDToInoreaderID: make(map[uuid.UUID]string, len(subscriptions)),
		LoadedAt:          startTime,
		TotalCount:        len(subscriptions),
	}

	for _, subscription := range subscriptions {
		mapping.InoreaderIDToUUID[subscription.InoreaderID] = subscription.DatabaseID
		mapping.UUIDToInoreaderID[subscription.DatabaseID] = subscription.InoreaderID
	}

	s.logger.Info("Subscription mapping cache built successfully",
		"subscription_count", mapping.TotalCount,
		"build_duration", time.Since(startTime))

	return mapping, nil
}

// ProcessArticleBatch processes a batch of articles, creating new ones and skipping duplicates
func (s *ArticleFetchService) ProcessArticleBatch(ctx context.Context, articles []*models.Article) (processed, skipped int, err error) {
	s.logger.Debug("Processing article batch", "count", len(articles))

	for _, article := range articles {
		// Skip articles with unknown subscriptions (Nil UUID)
		if article.SubscriptionID == uuid.Nil {
			s.logger.Debug("Skipping article with unknown subscription",
				"inoreader_id", article.InoreaderID,
				"title", article.Title)
			skipped++
			continue
		}

		// Check if article already exists
		existing, err := s.articleRepo.FindByInoreaderID(ctx, article.InoreaderID)
		if err != nil {
			// Article doesn't exist, create new one
			if createErr := s.articleRepo.Create(ctx, article); createErr != nil {
				s.logger.Error("Failed to create article",
					"inoreader_id", article.InoreaderID,
					"subscription_id", article.SubscriptionID,
					"error", createErr)
				return processed, skipped, fmt.Errorf("failed to create article %s: %w", article.InoreaderID, createErr)
			}

			s.logger.Debug("Created new article",
				"inoreader_id", article.InoreaderID,
				"subscription_id", article.SubscriptionID,
				"title", article.Title)
			processed++
		} else {
			// Article exists, skip it
			s.logger.Debug("Article already exists, skipping",
				"inoreader_id", article.InoreaderID,
				"existing_title", existing.Title)
			skipped++
		}
	}

	s.logger.Debug("Article batch processing completed",
		"processed", processed,
		"skipped", skipped,
		"total", len(articles))

	return processed, skipped, nil
}

// updateSyncState updates or creates sync state with new continuation token
func (s *ArticleFetchService) updateSyncState(ctx context.Context, streamID, continuationToken string, existingState *models.SyncState) error {
	if existingState == nil {
		// Create new sync state
		newState := models.NewSyncState(streamID, continuationToken)

		if err := s.syncStateRepo.Create(ctx, newState); err != nil {
			return fmt.Errorf("failed to create sync state: %w", err)
		}

		s.logger.Debug("Created new sync state",
			"stream_id", streamID,
			"continuation_token", continuationToken)
	} else {
		// Update existing sync state
		existingState.UpdateContinuationToken(continuationToken)

		if err := s.syncStateRepo.Update(ctx, existingState); err != nil {
			return fmt.Errorf("failed to update sync state: %w", err)
		}

		s.logger.Debug("Updated sync state",
			"stream_id", streamID,
			"continuation_token", continuationToken)
	}

	return nil
}

// GetUnprocessedArticles retrieves unprocessed articles from the database
func (s *ArticleFetchService) GetUnprocessedArticles(ctx context.Context, limit int) ([]*models.Article, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	articles, err := s.articleRepo.GetUnprocessed(ctx, limit)
	if err != nil {
		s.logger.Error("Failed to get unprocessed articles", "error", err, "limit", limit)
		return nil, fmt.Errorf("failed to get unprocessed articles: %w", err)
	}

	s.logger.Debug("Retrieved unprocessed articles",
		"count", len(articles),
		"limit", limit)

	return articles, nil
}

// MarkArticleAsProcessed marks an article as processed in the database
func (s *ArticleFetchService) MarkArticleAsProcessed(ctx context.Context, articleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.articleRepo.MarkAsProcessed(ctx, articleID); err != nil {
		s.logger.Error("Failed to mark article as processed", "error", err, "article_id", articleID)
		return fmt.Errorf("failed to mark article as processed: %w", err)
	}

	s.logger.Debug("Marked article as processed", "article_id", articleID)
	return nil
}

// DeleteOldArticles removes articles older than the specified time
func (s *ArticleFetchService) DeleteOldArticles(ctx context.Context, olderThan time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deletedCount, err := s.articleRepo.DeleteOld(ctx, olderThan)
	if err != nil {
		s.logger.Error("Failed to delete old articles", "error", err, "older_than", olderThan)
		return 0, fmt.Errorf("failed to delete old articles: %w", err)
	}

	s.logger.Info("Deleted old articles",
		"count", deletedCount,
		"older_than", olderThan)

	return deletedCount, nil
}
