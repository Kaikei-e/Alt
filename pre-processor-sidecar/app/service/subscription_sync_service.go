package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/models"
)

// SubscriptionRepository interface for subscription database operations
type SubscriptionRepository interface {
	FindByInoreaderID(ctx context.Context, inoreaderID string) (*models.Subscription, error)
	Create(ctx context.Context, subscription *models.Subscription) error
	Update(ctx context.Context, subscription *models.Subscription) error
	GetAll(ctx context.Context) ([]*models.Subscription, error)
	Delete(ctx context.Context, id string) error
}

// SubscriptionSyncResult represents the result of a subscription synchronization
type SubscriptionSyncResult struct {
	Created      int       `json:"created"`
	Updated      int       `json:"updated"`
	Deleted      int       `json:"deleted"`
	TotalProcessed int     `json:"total_processed"`
	SyncTime     time.Time `json:"sync_time"`
	Duration     time.Duration `json:"duration"`
	Errors       []string  `json:"errors,omitempty"`
}

// SubscriptionSyncStats represents synchronization statistics
type SubscriptionSyncStats struct {
	LastSyncTime     time.Time `json:"last_sync_time"`
	TotalSyncs       int64     `json:"total_syncs"`
	SuccessfulSyncs  int64     `json:"successful_syncs"`
	FailedSyncs      int64     `json:"failed_syncs"`
	Created          int       `json:"total_created"`
	Updated          int       `json:"total_updated"`
	Deleted          int       `json:"total_deleted"`
	LastError        string    `json:"last_error,omitempty"`
	NextSyncTime     time.Time `json:"next_sync_time"`
}

// SubscriptionSyncService handles synchronization of subscriptions from Inoreader API
type SubscriptionSyncService struct {
	inoreaderService    *InoreaderService
	subscriptionRepo    SubscriptionRepository
	logger              *slog.Logger
	syncInterval        time.Duration
	lastSyncTime        time.Time
	syncStats          *SubscriptionSyncStats
	mu                 sync.RWMutex
}

// NewSubscriptionSyncService creates a new subscription synchronization service
func NewSubscriptionSyncService(
	inoreaderService *InoreaderService, 
	subscriptionRepo SubscriptionRepository, 
	logger *slog.Logger,
) *SubscriptionSyncService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	return &SubscriptionSyncService{
		inoreaderService: inoreaderService,
		subscriptionRepo: subscriptionRepo,
		logger:          logger,
		syncInterval:    30 * time.Minute, // 30-minute sync interval as per plan
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

	// Process each subscription from Inoreader
	for _, inoreaderSub := range inoreaderSubscriptions {
		if err := s.processSubscription(ctx, inoreaderSub, result); err != nil {
			errorMsg := fmt.Sprintf("Failed to process subscription %s: %v", inoreaderSub.InoreaderID, err)
			s.logger.Error("Subscription processing error", 
				"inoreader_id", inoreaderSub.InoreaderID,
				"error", err)
			result.Errors = append(result.Errors, errorMsg)
		}
		result.TotalProcessed++
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

// processSubscription processes a single subscription from Inoreader API
func (s *SubscriptionSyncService) processSubscription(
	ctx context.Context, 
	inoreaderSub *models.Subscription, 
	result *SubscriptionSyncResult,
) error {
	// Check if subscription already exists in local database
	existingSub, err := s.subscriptionRepo.FindByInoreaderID(ctx, inoreaderSub.InoreaderID)
	if err != nil {
		// Subscription doesn't exist, create new one
		if err := s.subscriptionRepo.Create(ctx, inoreaderSub); err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}
		
		s.logger.Debug("Created new subscription",
			"inoreader_id", inoreaderSub.InoreaderID,
			"title", inoreaderSub.Title,
			"category", inoreaderSub.Category)
		
		result.Created++
		return nil
	}

	// Subscription exists, check if it needs updating
	if s.IsSubscriptionChanged(existingSub, inoreaderSub) {
		// Update existing subscription with new data
		existingSub.Title = inoreaderSub.Title
		existingSub.FeedURL = inoreaderSub.FeedURL
		existingSub.Category = inoreaderSub.Category
		existingSub.SyncedAt = time.Now()

		if err := s.subscriptionRepo.Update(ctx, existingSub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}

		s.logger.Debug("Updated existing subscription",
			"inoreader_id", inoreaderSub.InoreaderID,
			"title", inoreaderSub.Title,
			"category", inoreaderSub.Category)

		result.Updated++
	} else {
		s.logger.Debug("Subscription unchanged, skipping",
			"inoreader_id", inoreaderSub.InoreaderID)
	}

	return nil
}

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