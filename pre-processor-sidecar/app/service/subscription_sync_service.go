// ABOUTME: Dedicated service for subscription synchronization logic
// ABOUTME: Handles subscription updates, caching, and UUID mapping for articles

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

// Note: SubscriptionRepository interface is defined in repository package

// SubscriptionSyncResult represents the result of a subscription synchronization
type SubscriptionSyncResult struct {
	Created        int           `json:"created"`
	Updated        int           `json:"updated"`
	Deleted        int           `json:"deleted"`
	TotalProcessed int           `json:"total_processed"`
	SyncTime       time.Time     `json:"sync_time"`
	Duration       time.Duration `json:"duration"`
	Errors         []string      `json:"errors,omitempty"`
}

// SubscriptionSyncStats represents synchronization statistics
type SubscriptionSyncStats struct {
	LastSyncTime    time.Time `json:"last_sync_time"`
	TotalSyncs      int64     `json:"total_syncs"`
	SuccessfulSyncs int64     `json:"successful_syncs"`
	FailedSyncs     int64     `json:"failed_syncs"`
	Created         int       `json:"total_created"`
	Updated         int       `json:"total_updated"`
	Deleted         int       `json:"total_deleted"`
	LastError       string    `json:"last_error,omitempty"`
	NextSyncTime    time.Time `json:"next_sync_time"`
}

// SubscriptionSyncService handles subscription synchronization and UUID mapping
type SubscriptionSyncService struct {
	inoreaderService     *InoreaderService
	subscriptionRepo     repository.SubscriptionRepository
	syncRepo             repository.SyncStateRepository // Added SyncStateRepository
	logger               *slog.Logger
	subscriptionCache    map[string]uuid.UUID // InoreaderID -> UUID mapping
	cacheLastUpdated     time.Time
	cacheMutex           sync.RWMutex
	syncInterval         time.Duration
	cacheRefreshInterval time.Duration
	lastSyncTime         time.Time
	syncStats            *SubscriptionSyncStats
	mu                   sync.RWMutex
}

// NewSubscriptionSyncService creates a new subscription synchronization service
func NewSubscriptionSyncService(
	inoreaderService *InoreaderService,
	subscriptionRepo repository.SubscriptionRepository,
	syncRepo repository.SyncStateRepository, // Copied SyncStateRepository parameter
	logger *slog.Logger,
) *SubscriptionSyncService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	return &SubscriptionSyncService{
		inoreaderService:     inoreaderService,
		subscriptionRepo:     subscriptionRepo,
		syncRepo:             syncRepo, // Initialized SyncStateRepository
		logger:               logger,
		syncInterval:         4 * time.Hour, // 4-hour sync interval as requested
		subscriptionCache:    make(map[string]uuid.UUID),
		cacheRefreshInterval: 30 * time.Minute, // Refresh cache every 30 minutes
		syncStats: &SubscriptionSyncStats{
			TotalSyncs:      0,
			SuccessfulSyncs: 0,
			FailedSyncs:     0,
			Created:         0,
			Updated:         0,
			Deleted:         0,
		},
	}
}

// SyncSubscriptions synchronizes subscriptions from Inoreader API to local database
func (s *SubscriptionSyncService) SyncSubscriptions(ctx context.Context) (*SubscriptionSyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	startTime := time.Now()
	s.logger.Info("Starting subscription synchronization",
		"sync_interval", s.syncInterval,
		"last_sync", s.lastSyncTime)

	// Update sync stats
	s.syncStats.TotalSyncs++

	result := &SubscriptionSyncResult{
		SyncTime:       startTime,
		TotalProcessed: 0,
		Errors:         []string{},
	}

	// Fetch current subscriptions from Inoreader API
	inoreaderSubscriptions, err := s.inoreaderService.FetchSubscriptions(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch subscriptions from Inoreader API", "error", err)
		s.syncStats.FailedSyncs++
		s.syncStats.LastError = err.Error()
		return nil, fmt.Errorf("failed to fetch subscriptions from Inoreader: %w", err)
	}

	s.logger.Info("Fetched subscriptions from Inoreader API",
		"count", len(inoreaderSubscriptions))

	// Get existing subscriptions from database to determine what's new vs updated
	existingSubscriptions, err := s.subscriptionRepo.GetAllSubscriptions(ctx)
	if err != nil {
		s.logger.Error("Failed to get existing subscriptions from database", "error", err)
		s.syncStats.FailedSyncs++
		s.syncStats.LastError = err.Error()
		return nil, fmt.Errorf("failed to get existing subscriptions: %w", err)
	}

	// Create a map of existing subscriptions by InoreaderID for quick lookup
	existingMap := make(map[string]models.InoreaderSubscription)
	for _, sub := range existingSubscriptions {
		existingMap[sub.InoreaderID] = sub
	}

	// Determine what's new vs updated
	created := 0
	updated := 0

	for _, subscription := range inoreaderSubscriptions {
		if existing, exists := existingMap[subscription.InoreaderID]; exists {
			// Check if subscription has changed (comparing simplified Subscription with InoreaderSubscription)
			existingCategory := ""
			if len(existing.Categories) > 0 {
				existingCategory = existing.Categories[0].Label
			}

			if existing.Title != subscription.Title ||
				existing.URL != subscription.FeedURL ||
				existingCategory != subscription.Category {
				updated++
			}
		} else {
			created++
		}
	}

	// Use the new SyncSubscriptionsNew method to perform the actual sync
	if err := s.SyncSubscriptionsNew(ctx); err != nil {
		errorMsg := fmt.Sprintf("Failed to sync subscriptions: %v", err)
		s.logger.Error("Subscription sync error", "error", err)
		result.Errors = append(result.Errors, errorMsg)
		result.TotalProcessed = len(inoreaderSubscriptions)
	} else {
		result.Created = created
		result.Updated = updated
		result.TotalProcessed = len(inoreaderSubscriptions)
	}

	// Update sync completion stats
	result.Duration = time.Since(startTime)
	s.lastSyncTime = startTime
	s.syncStats.LastSyncTime = startTime
	s.syncStats.NextSyncTime = startTime.Add(s.syncInterval)

	if len(result.Errors) == 0 {
		s.syncStats.SuccessfulSyncs++
		s.syncStats.LastError = ""
	} else {
		s.syncStats.FailedSyncs++
		s.syncStats.LastError = fmt.Sprintf("%d errors occurred during sync", len(result.Errors))
	}

	// Update cumulative stats
	s.syncStats.Created += result.Created
	s.syncStats.Updated += result.Updated
	s.syncStats.Deleted += result.Deleted

	s.logger.Info("Subscription synchronization completed",
		"duration", result.Duration,
		"created", result.Created,
		"updated", result.Updated,
		"deleted", result.Deleted,
		"total_processed", result.TotalProcessed,
		"errors", len(result.Errors))

	return result, nil
}

// processSubscription is deprecated - use SyncSubscriptionsNew instead

// IsSubscriptionChanged compares two subscriptions to determine if changes occurred
func (s *SubscriptionSyncService) IsSubscriptionChanged(existing, incoming *models.Subscription) bool {
	return existing.Title != incoming.Title ||
		existing.FeedURL != incoming.FeedURL ||
		existing.Category != incoming.Category
}

// GetSyncStats returns current synchronization statistics
func (s *SubscriptionSyncService) GetSyncStats() *SubscriptionSyncStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to avoid race conditions
	statsCopy := *s.syncStats
	return &statsCopy
}

// GetLastSyncTime returns the timestamp of the last successful sync
func (s *SubscriptionSyncService) GetLastSyncTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTime
}

// GetNextSyncTime returns the estimated time of the next synchronization
func (s *SubscriptionSyncService) GetNextSyncTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTime.Add(s.syncInterval)
}

// SetSyncInterval updates the synchronization interval
func (s *SubscriptionSyncService) SetSyncInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syncInterval = interval
	s.logger.Info("Sync interval updated", "new_interval", interval)
}

// IsReadyForSync checks if enough time has passed since last sync
func (s *SubscriptionSyncService) IsReadyForSync() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lastSyncTime.IsZero() {
		return true // First sync
	}

	return time.Since(s.lastSyncTime) >= s.syncInterval
}

// ResetStats resets synchronization statistics
func (s *SubscriptionSyncService) ResetStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syncStats = &SubscriptionSyncStats{
		TotalSyncs:      0,
		SuccessfulSyncs: 0,
		FailedSyncs:     0,
		Created:         0,
		Updated:         0,
		Deleted:         0,
		LastError:       "",
	}

	s.logger.Info("Synchronization statistics reset")
}

// RefreshSubscriptionCache refreshes the in-memory subscription cache for UUID mapping
func (s *SubscriptionSyncService) RefreshSubscriptionCache(ctx context.Context) error {
	s.logger.Debug("Refreshing subscription cache")

	// Get all subscriptions from database
	subscriptions, err := s.subscriptionRepo.GetAllSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("failed to get subscriptions from database: %w", err)
	}

	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	// Clear and rebuild cache
	s.subscriptionCache = make(map[string]uuid.UUID)
	for _, subscription := range subscriptions {
		s.subscriptionCache[subscription.InoreaderID] = subscription.DatabaseID // 修正: DatabaseIDを使用
	}

	s.cacheLastUpdated = time.Now()

	s.logger.Info("Subscription cache refreshed successfully",
		"cache_size", len(s.subscriptionCache),
		"last_updated", s.cacheLastUpdated)

	return nil
}

// GetSubscriptionUUID returns the UUID for a given Inoreader subscription ID
func (s *SubscriptionSyncService) GetSubscriptionUUID(inoreaderID string) (uuid.UUID, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Check if cache needs refresh
	if time.Since(s.cacheLastUpdated) > s.cacheRefreshInterval {
		s.cacheMutex.RUnlock()
		s.logger.Debug("Subscription cache expired, triggering refresh",
			"last_updated", s.cacheLastUpdated,
			"refresh_interval", s.cacheRefreshInterval)

		// Try to refresh cache (async to avoid blocking)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.RefreshSubscriptionCache(ctx); err != nil {
				s.logger.Error("Async cache refresh failed", "error", err)
			}
		}()

		s.cacheMutex.RLock()
	}

	subscriptionUUID, exists := s.subscriptionCache[inoreaderID]
	if !exists {
		s.logger.Debug("Subscription UUID not found in cache",
			"inoreader_id", inoreaderID,
			"cache_size", len(s.subscriptionCache))
	}

	return subscriptionUUID, exists
}

// ResolveArticleSubscriptionUUIDs resolves OriginStreamID to SubscriptionID for articles
func (s *SubscriptionSyncService) ResolveArticleSubscriptionUUIDs(articles []*models.Article) []*models.Article {
	resolvedCount := 0
	unresolvedCount := 0

	for _, article := range articles {
		if article.OriginStreamID != "" {
			if subscriptionUUID, found := s.GetSubscriptionUUID(article.OriginStreamID); found {
				article.SubscriptionID = subscriptionUUID
				resolvedCount++
				s.logger.Debug("Resolved article subscription UUID",
					"inoreader_id", article.InoreaderID,
					"origin_stream_id", article.OriginStreamID,
					"subscription_id", subscriptionUUID)
			} else {
				unresolvedCount++
				s.logger.Debug("Could not resolve article subscription UUID",
					"inoreader_id", article.InoreaderID,
					"origin_stream_id", article.OriginStreamID)
			}
		}
	}

	s.logger.Info("Article UUID resolution completed",
		"total_articles", len(articles),
		"resolved", resolvedCount,
		"unresolved", unresolvedCount)

	return articles
}

// GetCacheStatus returns subscription cache statistics for monitoring
func (s *SubscriptionSyncService) GetCacheStatus() map[string]interface{} {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	return map[string]interface{}{
		"cache_size":        len(s.subscriptionCache),
		"last_updated":      s.cacheLastUpdated,
		"refresh_interval":  s.cacheRefreshInterval,
		"cache_age_minutes": int(time.Since(s.cacheLastUpdated).Minutes()),
		"needs_refresh":     time.Since(s.cacheLastUpdated) > s.cacheRefreshInterval,
	}
}

// InitializeCache initializes the subscription cache on service startup
func (s *SubscriptionSyncService) InitializeCache(ctx context.Context) error {
	s.logger.Info("Initializing subscription cache")

	if err := s.RefreshSubscriptionCache(ctx); err != nil {
		return fmt.Errorf("failed to initialize subscription cache: %w", err)
	}

	s.logger.Info("Subscription cache initialized successfully")
	return nil
}

// SyncSubscriptionsNew performs modern subscription sync with caching
func (s *SubscriptionSyncService) SyncSubscriptionsNew(ctx context.Context) error {
	s.logger.Info("Starting subscription synchronization (new method)")

	// Fetch subscriptions from Inoreader API
	subscriptions, err := s.inoreaderService.FetchSubscriptions(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch subscriptions from Inoreader API", "error", err)
		return fmt.Errorf("subscription fetch failed: %w", err)
	}

	if len(subscriptions) == 0 {
		s.logger.Warn("No subscriptions retrieved from Inoreader API")
		return nil
	}

	// Convert to repository format
	repoSubscriptions := s.convertToRepositoryFormat(subscriptions)

	// Save to database
	if err := s.subscriptionRepo.SaveSubscriptions(ctx, repoSubscriptions); err != nil {
		s.logger.Error("Failed to save subscriptions to database", "error", err)
		return fmt.Errorf("subscription save failed: %w", err)
	}

	// Ensure sync states exist for all subscriptions
	createdSyncStates := 0
	for _, sub := range subscriptions {
		// Check if sync state already exists
		existingSyncState, err := s.syncRepo.FindByStreamID(ctx, sub.InoreaderID)
		if err != nil {
			// FindByStreamID returns error if not found? No, it returns error if query fails
			// Check implementation of FindByStreamID in repo
			// It returns nil, fmt.Errorf("sync state not found... ") usually if it's strict
			// Let's assume standard repo pattern check
			// Actually looked at implementation:
			// if err == sql.ErrNoRows { return nil, fmt.Errorf(...) }
			// So it returns error if not found.
			// We need to check if error string contains "not found" or specific error type if available.
			// But simpler approach: if Create fails on duplicate key, it's fine too if we handle it.
			// However better to try find first.
			// Wait, the repo implementation returns error on not found.
			// "sync state not found for stream_id: %s"
			// So err != nil means either not found or DB error.
			s.logger.Debug("Sync state check", "stream_id", sub.InoreaderID, "error_checking", err)
		}

		if existingSyncState == nil {
			// Create new sync state
			newSyncState := models.NewSyncState(sub.InoreaderID, "")
			if err := s.syncRepo.Create(ctx, newSyncState); err != nil {
				s.logger.Error("Failed to create sync state", "stream_id", sub.InoreaderID, "error", err)
			} else {
				createdSyncStates++
			}
		}
	}

	s.logger.Info("Successfully synchronized subscriptions",
		"count", len(subscriptions),
		"sync_interval", s.syncInterval,
		"new_sycn_states", createdSyncStates)

	// Update subscription cache
	if err := s.RefreshSubscriptionCache(ctx); err != nil {
		s.logger.Warn("Failed to refresh subscription cache after sync", "error", err)
		// Don't return error as sync was successful
	}

	return nil
}

// convertToRepositoryFormat converts service models to repository format
func (s *SubscriptionSyncService) convertToRepositoryFormat(subscriptions []*models.Subscription) []models.InoreaderSubscription {
	repoSubscriptions := make([]models.InoreaderSubscription, 0, len(subscriptions))

	for _, subscription := range subscriptions {
		repoSub := models.InoreaderSubscription{
			DatabaseID:  subscription.ID, // 修正: DatabaseIDフィールドを使用
			InoreaderID: subscription.InoreaderID,
			URL:         subscription.FeedURL, // Use URL instead of FeedURL
			Title:       subscription.Title,
			Categories:  []models.InoreaderCategory{{Label: subscription.Category}}, // Convert to Categories slice
			CreatedAt:   subscription.CreatedAt,
			UpdatedAt:   time.Now(),
		}
		repoSubscriptions = append(repoSubscriptions, repoSub)
	}

	return repoSubscriptions
}
