//go:generate mockgen -source=article_fetch_service.go -destination=../mocks/article_fetch_repositories_mock.go -package=mocks ArticleRepository,SyncStateRepository

package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/domain"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/usecase"

	"github.com/google/uuid"
)

// ArticleRepository interface for article database operations
type ArticleRepository interface {
	FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Article, error)
	Create(ctx context.Context, article *models.Article) error
	CreateBatch(ctx context.Context, articles []*models.Article) (int, error)
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
	inoreaderService     *InoreaderService
	articleRepo          ArticleRepository
	syncStateRepo        SyncStateRepository
	subscriptionRepo     repository.SubscriptionRepository // Added for UUID resolution
	uuidResolutionUseCase *usecase.ArticleUUIDResolutionUseCase // Clean Architecture UUID resolution
	logger               *slog.Logger
	mu                   sync.RWMutex
}

// SlogAdapter adapts slog.Logger to domain.LoggerInterface
type SlogAdapter struct {
	logger *slog.Logger
}

func (a *SlogAdapter) Info(msg string, args ...interface{}) {
	a.logger.Info(msg, args...)
}

func (a *SlogAdapter) Warn(msg string, args ...interface{}) {
	a.logger.Warn(msg, args...)
}

func (a *SlogAdapter) Error(msg string, args ...interface{}) {
	a.logger.Error(msg, args...)
}

func (a *SlogAdapter) Debug(msg string, args ...interface{}) {
	a.logger.Debug(msg, args...)
}

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

	// Create Clean Architecture components
	loggerAdapter := &SlogAdapter{logger: logger}
	autoCreatorAdapter := usecase.NewSubscriptionAutoCreatorAdapter(subscriptionRepo, loggerAdapter)
	uuidResolver := domain.NewSubscriptionUUIDResolver(autoCreatorAdapter, loggerAdapter)
	uuidResolutionUseCase := usecase.NewArticleUUIDResolutionUseCase(uuidResolver, subscriptionRepo, loggerAdapter)

	return &ArticleFetchService{
		inoreaderService:      inoreaderService,
		articleRepo:           articleRepo,
		syncStateRepo:         syncStateRepo,
		subscriptionRepo:      subscriptionRepo,
		uuidResolutionUseCase: uuidResolutionUseCase,
		logger:                logger,
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

	// DEPRECATED: Old subscription mapping approach - now handled by Clean Architecture use case

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

	// Step 4: Resolve subscription UUIDs using Clean Architecture use case
	s.logger.Info("Starting Clean Architecture UUID resolution",
		"total_articles", len(articles))
	
	uuidResult, err := s.uuidResolutionUseCase.ResolveArticleUUIDs(ctx, articles)
	if err != nil {
		s.logger.Error("Failed to resolve article UUIDs with Clean Architecture", "error", err)
		return nil, fmt.Errorf("failed to resolve article UUIDs: %w", err)
	}

	s.logger.Info("Clean Architecture UUID resolution completed",
		"total_articles", uuidResult.TotalProcessed,
		"resolved", uuidResult.ResolvedCount,
		"auto_created", uuidResult.AutoCreatedCount,
		"unknown", uuidResult.UnknownCount,
		"errors", len(uuidResult.Errors))

	// Log errors for debugging if any occurred
	for _, resolutionError := range uuidResult.Errors {
		s.logger.Error("UUID resolution error",
			"article_inoreader_id", resolutionError.ArticleInoreaderID,
			"origin_stream_id", resolutionError.OriginStreamID,
			"error_code", resolutionError.ErrorCode,
			"error_message", resolutionError.ErrorMessage)
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

// DEPRECATED: buildSubscriptionMapping is now handled by ArticleUUIDResolutionUseCase
// Keeping the old SubscriptionMapping struct for backward compatibility in other methods

// DEPRECATED: autoCreateSubscription, extractFeedURLFromInoreaderID, generateAutoTitle 
// are now handled by SubscriptionAutoCreatorAdapter in the use case layer

// ProcessArticleBatch processes a batch of articles with auto-subscription creation
func (s *ArticleFetchService) ProcessArticleBatch(ctx context.Context, articles []*models.Article) (processed, skipped int, err error) {
	s.logger.Info("Starting resilient article batch processing", 
		"total_articles", len(articles))

	processed = 0
	skipped = 0
	
	// Use CreateBatch for resilient processing (individual transactions)
	createdCount, batchErr := s.articleRepo.CreateBatch(ctx, articles)
	if batchErr != nil {
		s.logger.Error("Batch processing failed completely", "error", batchErr)
		return 0, len(articles), fmt.Errorf("article batch processing failed: %w", batchErr)
	}

	processed = createdCount
	skipped = len(articles) - createdCount

	s.logger.Info("Resilient article batch processing completed",
		"total_articles", len(articles),
		"processed", processed,
		"skipped", skipped,
		"success_rate", fmt.Sprintf("%.1f%%", float64(processed)/float64(len(articles))*100))

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
