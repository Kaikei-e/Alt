// ABOUTME: Scheduling handler for managing dual schedule processing
// ABOUTME: Handles subscription sync (4 hours) and article fetch (30 minutes) schedules

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"pre-processor-sidecar/service"
)

// ScheduleConfig represents scheduling configuration
type ScheduleConfig struct {
	SubscriptionSyncInterval time.Duration `json:"subscription_sync_interval"` // 4 hours
	ArticleFetchInterval     time.Duration `json:"article_fetch_interval"`     // 30 minutes
	EnableSubscriptionSync   bool          `json:"enable_subscription_sync"`
	EnableArticleFetch       bool          `json:"enable_article_fetch"`
	MaxConcurrentJobs        int           `json:"max_concurrent_jobs"`
	EnableRandomStart        bool          `json:"enable_random_start"`        // Enable random starting position for rotation
}

// RateLimitAwareScheduler implements intelligent scheduling with exponential backoff
type RateLimitAwareScheduler struct {
	baseInterval      time.Duration
	currentInterval   time.Duration
	errorCount        int
	lastSuccessTime   time.Time
	backoffMultiplier float64
	maxInterval       time.Duration
	mu                sync.Mutex
}

// NewRateLimitAwareScheduler creates a new intelligent scheduler
func NewRateLimitAwareScheduler(baseInterval time.Duration) *RateLimitAwareScheduler {
	return &RateLimitAwareScheduler{
		baseInterval:      baseInterval,
		currentInterval:   baseInterval,
		backoffMultiplier: 1.5,
		maxInterval:       6 * time.Hour, // Max 6 hours backoff
	}
}

// NextInterval calculates the next execution interval with exponential backoff
func (s *RateLimitAwareScheduler) NextInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.errorCount == 0 {
		s.currentInterval = s.baseInterval
		return s.currentInterval
	}

	// Exponential backoff calculation
	backoffDuration := time.Duration(
		float64(s.baseInterval) *
			math.Pow(s.backoffMultiplier, float64(s.errorCount)),
	)

	if backoffDuration > s.maxInterval {
		backoffDuration = s.maxInterval
	}

	s.currentInterval = backoffDuration
	return s.currentInterval
}

// RecordSuccess resets the error count and updates last success time
func (s *RateLimitAwareScheduler) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorCount = 0
	s.lastSuccessTime = time.Now()
	s.currentInterval = s.baseInterval
}

// RecordError increments error count for backoff calculation
func (s *RateLimitAwareScheduler) RecordError() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorCount++
}

// GetStatus returns current scheduler status
func (s *RateLimitAwareScheduler) GetStatus() (int, time.Duration, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.errorCount, s.currentInterval, s.lastSuccessTime
}

// ScheduleStatus represents current scheduling status
type ScheduleStatus struct {
	SubscriptionSyncEnabled bool      `json:"subscription_sync_enabled"`
	ArticleFetchEnabled     bool      `json:"article_fetch_enabled"`
	LastSubscriptionSync    time.Time `json:"last_subscription_sync"`
	NextSubscriptionSync    time.Time `json:"next_subscription_sync"`
	LastArticleFetch        time.Time `json:"last_article_fetch"`
	NextArticleFetch        time.Time `json:"next_article_fetch"`
	SubscriptionSyncRunning bool      `json:"subscription_sync_running"`
	ArticleFetchRunning     bool      `json:"article_fetch_running"`
	TotalSubscriptionSyncs  int64     `json:"total_subscription_syncs"`
	TotalArticleFetches     int64     `json:"total_article_fetches"`
	FailedSubscriptionSyncs int64     `json:"failed_subscription_syncs"`
	FailedArticleFetches    int64     `json:"failed_article_fetches"`
	LastError               string    `json:"last_error,omitempty"`
}

// JobResult represents the result of a scheduled job
type JobResult struct {
	JobType   string        `json:"job_type"` // "subscription_sync" or "article_fetch"
	Success   bool          `json:"success"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Details   interface{}   `json:"details,omitempty"`
}

// ScheduleHandler manages dual schedule processing for subscriptions and articles
type ScheduleHandler struct {
	config              *ScheduleConfig
	articleFetchHandler *ArticleFetchHandler         // Legacy handler for subscription sync
	articleFetchService *service.ArticleFetchService // New service for rotation processing
	status              *ScheduleStatus
	logger              *slog.Logger
	subscriptionTicker  *time.Ticker
	articleFetchTicker  *time.Ticker
	ctx                 context.Context
	cancel              context.CancelFunc
	mu                  sync.RWMutex
	jobResultCallbacks  []func(*JobResult)
	// Intelligent schedulers with rate limit awareness
	subscriptionScheduler *RateLimitAwareScheduler
	articleFetchScheduler *RateLimitAwareScheduler
}

// NewScheduleHandler creates a new schedule handler with rotation support
func NewScheduleHandler(
	articleFetchHandler *ArticleFetchHandler,
	articleFetchService *service.ArticleFetchService,
	logger *slog.Logger,
) *ScheduleHandler {
	if logger == nil {
		logger = slog.Default()
	}

	// Default configuration - API optimized with 30-minute intervals
	config := &ScheduleConfig{
		SubscriptionSyncInterval: 12 * time.Hour,   // 12 hours for subscription sync (API optimized)
		ArticleFetchInterval:     30 * time.Minute, // 30-minute intervals for article fetch
		EnableSubscriptionSync:   true,
		EnableArticleFetch:       true,
		MaxConcurrentJobs:        2, // Allow subscription sync and article fetch to run concurrently
		EnableRandomStart:        true, // Enable random starting position for fair load distribution
	}

	status := &ScheduleStatus{
		SubscriptionSyncEnabled: config.EnableSubscriptionSync,
		ArticleFetchEnabled:     config.EnableArticleFetch,
		NextSubscriptionSync:    time.Now().Add(config.SubscriptionSyncInterval),
		NextArticleFetch:        time.Now().Add(config.ArticleFetchInterval),
	}

	return &ScheduleHandler{
		config:                config,
		articleFetchHandler:   articleFetchHandler,
		articleFetchService:   articleFetchService,
		status:                status,
		logger:                logger,
		jobResultCallbacks:    make([]func(*JobResult), 0),
		subscriptionScheduler: NewRateLimitAwareScheduler(config.SubscriptionSyncInterval),
		articleFetchScheduler: NewRateLimitAwareScheduler(config.ArticleFetchInterval),
	}
}

// Start starts the dual schedule processing
func (h *ScheduleHandler) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ctx != nil {
		return fmt.Errorf("schedule handler already running")
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	h.logger.Info("Starting dual schedule processing",
		"subscription_sync_interval", h.config.SubscriptionSyncInterval,
		"article_fetch_interval", h.config.ArticleFetchInterval,
		"subscription_sync_enabled", h.config.EnableSubscriptionSync,
		"article_fetch_enabled", h.config.EnableArticleFetch)

	// Start subscription sync scheduler
	if h.config.EnableSubscriptionSync {
		h.subscriptionTicker = time.NewTicker(h.config.SubscriptionSyncInterval)
		go h.runSubscriptionSyncScheduler()
		h.logger.Info("Subscription sync scheduler started",
			"interval", h.config.SubscriptionSyncInterval)
	}

	// Start article fetch scheduler with rotation processing
	if h.config.EnableArticleFetch {
		// Enable rotation mode on ArticleFetchService with random start if configured
		var err error
		if h.config.EnableRandomStart {
			err = h.articleFetchService.EnableRotationModeWithRandomStart(ctx)
			h.logger.Info("Article fetch rotation mode enabled with random start")
		} else {
			err = h.articleFetchService.EnableRotationMode(ctx)
			h.logger.Info("Article fetch rotation mode enabled")
		}
		
		if err != nil {
			h.logger.Error("Failed to enable rotation mode", "error", err)
			return fmt.Errorf("failed to enable rotation mode: %w", err)
		}

		h.articleFetchTicker = time.NewTicker(h.config.ArticleFetchInterval)
		go h.runArticleFetchScheduler()
		h.logger.Info("Article fetch scheduler started with rotation processing",
			"interval", h.config.ArticleFetchInterval)
	}

	// Run initial jobs after a short delay
	go func() {
		time.Sleep(30 * time.Second) // Wait 30 seconds after startup

		if h.config.EnableSubscriptionSync {
			h.executeSubscriptionSync()
		}

		time.Sleep(1 * time.Minute) // Wait another minute before article fetch

		if h.config.EnableArticleFetch {
			h.executeArticleFetch()
		}
	}()

	return nil
}

// Stop stops the dual schedule processing
func (h *ScheduleHandler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
	}

	if h.subscriptionTicker != nil {
		h.subscriptionTicker.Stop()
		h.subscriptionTicker = nil
	}

	if h.articleFetchTicker != nil {
		h.articleFetchTicker.Stop()
		h.articleFetchTicker = nil
	}

	h.logger.Info("Dual schedule processing stopped")
}

// runSubscriptionSyncScheduler runs the subscription sync scheduler
func (h *ScheduleHandler) runSubscriptionSyncScheduler() {
	h.logger.Info("Subscription sync scheduler started")

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("Subscription sync scheduler stopped")
			return
		case <-h.subscriptionTicker.C:
			if h.config.EnableSubscriptionSync && !h.status.SubscriptionSyncRunning {
				go h.executeSubscriptionSync()
			}
		}
	}
}

// runArticleFetchScheduler runs the article fetch scheduler with dynamic intervals
func (h *ScheduleHandler) runArticleFetchScheduler() {
	h.logger.Info("Article fetch scheduler started with dynamic interval adjustment")

	// Use dynamic timer instead of fixed ticker
	nextInterval := h.articleFetchScheduler.NextInterval()
	timer := time.NewTimer(nextInterval)
	defer timer.Stop()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info("Article fetch scheduler stopped")
			return
		case <-timer.C:
			if h.config.EnableArticleFetch && !h.status.ArticleFetchRunning {
				go func() {
					h.executeArticleFetch()

					// Reset timer with updated interval after execution
					h.mu.RLock()
					nextInterval := h.articleFetchScheduler.NextInterval()
					errorCount, _, lastSuccess := h.articleFetchScheduler.GetStatus()
					h.mu.RUnlock()

					h.logger.Debug("Rescheduling article fetch",
						"next_interval", nextInterval,
						"error_count", errorCount,
						"last_success", lastSuccess)

					timer.Reset(nextInterval)
				}()
			} else {
				// Reset timer even if skipped
				nextInterval := h.articleFetchScheduler.NextInterval()
				timer.Reset(nextInterval)
			}
		}
	}
}

// executeSubscriptionSync executes subscription synchronization
func (h *ScheduleHandler) executeSubscriptionSync() {
	h.mu.Lock()
	if h.status.SubscriptionSyncRunning {
		h.mu.Unlock()
		h.logger.Warn("Subscription sync already running, skipping")
		return
	}
	h.status.SubscriptionSyncRunning = true
	h.status.TotalSubscriptionSyncs++
	h.mu.Unlock()

	startTime := time.Now()
	result := &JobResult{
		JobType:   "subscription_sync",
		StartTime: startTime,
	}

	h.logger.Info("Starting scheduled subscription synchronization")

	// Check if context is available
	if h.ctx == nil {
		h.logger.Warn("Handler context not initialized, skipping subscription sync")
		result.Success = false
		result.Error = "handler context not initialized"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		h.mu.Lock()
		h.status.SubscriptionSyncRunning = false
		h.mu.Unlock()
		h.notifyJobResult(result)
		return
	}

	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Minute) // 10-minute timeout
	defer cancel()

	err := h.articleFetchHandler.ExecuteSubscriptionSync(ctx)

	endTime := time.Now()
	result.EndTime = endTime
	result.Duration = endTime.Sub(startTime)
	result.Success = err == nil

	h.mu.Lock()
	h.status.SubscriptionSyncRunning = false
	h.status.LastSubscriptionSync = endTime
	h.status.NextSubscriptionSync = endTime.Add(h.config.SubscriptionSyncInterval)

	if err != nil {
		result.Error = err.Error()
		h.status.FailedSubscriptionSyncs++
		h.status.LastError = err.Error()
		h.logger.Error("Scheduled subscription sync failed",
			"duration", result.Duration,
			"error", err)
	} else {
		h.logger.Info("Scheduled subscription sync completed successfully",
			"duration", result.Duration,
			"next_sync", h.status.NextSubscriptionSync)
	}
	h.mu.Unlock()

	// Notify callbacks
	h.notifyJobResult(result)
}

// executeArticleFetch executes article fetching with rotation processing
func (h *ScheduleHandler) executeArticleFetch() {
	h.mu.Lock()
	if h.status.ArticleFetchRunning {
		h.mu.Unlock()
		h.logger.Warn("Article fetch already running, skipping")
		return
	}
	h.status.ArticleFetchRunning = true
	h.status.TotalArticleFetches++
	h.mu.Unlock()

	startTime := time.Now()
	result := &JobResult{
		JobType:   "article_fetch",
		StartTime: startTime,
	}

	h.logger.Info("Starting scheduled article fetching with rotation processing")

	// Check if context is available
	if h.ctx == nil {
		h.logger.Warn("Handler context not initialized, skipping article fetch")
		result.Success = false
		result.Error = "handler context not initialized"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		h.notifyJobResult(result)
		h.mu.Lock()
		h.status.ArticleFetchRunning = false
		h.mu.Unlock()
		return
	}

	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Minute) // 10-minute timeout for single subscription
	defer cancel()

	// Use rotation processing instead of batch processing
	err := h.processNextSubscriptionRotation(ctx, result)

	endTime := time.Now()
	result.EndTime = endTime
	result.Duration = endTime.Sub(startTime)
	result.Success = err == nil

	h.mu.Lock()
	h.status.ArticleFetchRunning = false
	h.status.LastArticleFetch = endTime

	if err != nil {
		result.Error = err.Error()
		h.status.FailedArticleFetches++
		h.status.LastError = err.Error()

		// Record error in intelligent scheduler for backoff calculation
		h.articleFetchScheduler.RecordError()
		nextInterval := h.articleFetchScheduler.NextInterval()
		h.status.NextArticleFetch = endTime.Add(nextInterval)

		errorCount, _, lastSuccess := h.articleFetchScheduler.GetStatus()
		h.logger.Error("Scheduled article fetch failed - applying intelligent backoff",
			"duration", result.Duration,
			"error", err,
			"consecutive_errors", errorCount,
			"next_interval", nextInterval,
			"last_success", lastSuccess)
	} else {
		// Record success in intelligent scheduler to reset backoff
		h.articleFetchScheduler.RecordSuccess()
		h.status.NextArticleFetch = endTime.Add(h.config.ArticleFetchInterval)

		rotationStats := h.articleFetchService.GetRotationStats()
		h.logger.Info("Scheduled article fetch completed successfully with rotation",
			"duration", result.Duration,
			"processed_today", rotationStats.ProcessedToday,
			"remaining_today", rotationStats.RemainingToday,
			"total_subscriptions", rotationStats.TotalSubscriptions,
			"next_processing", rotationStats.NextProcessingTime.Format("15:04:05"),
			"estimated_completion", rotationStats.EstimatedCompletionTime.Format("15:04:05"),
			"next_fetch", h.status.NextArticleFetch)
	}
	h.mu.Unlock()

	// Notify callbacks
	h.notifyJobResult(result)
}

// processNextSubscriptionRotation processes the next subscription in the rotation queue
func (h *ScheduleHandler) processNextSubscriptionRotation(ctx context.Context, result *JobResult) error {
	// Check if rotation processing is ready
	if !h.articleFetchService.IsRotationEnabled() {
		h.logger.Warn("Rotation mode not enabled, attempting to enable")
		
		var err error
		if h.config.EnableRandomStart {
			err = h.articleFetchService.EnableRotationModeWithRandomStart(ctx)
			h.logger.Info("Rotation mode enabled with random start for subscription processing")
		} else {
			err = h.articleFetchService.EnableRotationMode(ctx)
		}
		
		if err != nil {
			return fmt.Errorf("failed to enable rotation mode: %w", err)
		}
	}

	// Get rotation statistics before processing
	statsBefore := h.articleFetchService.GetRotationStats()
	
	// デバッグ情報をログに出力
	timezoneInfo := h.articleFetchService.GetRotatorTimezoneInfo()
	h.logger.Info("Rotation stats before processing",
		"processed_today", statsBefore.ProcessedToday,
		"remaining_today", statsBefore.RemainingToday,
		"total_subscriptions", statsBefore.TotalSubscriptions,
		"current_time", time.Now().Format(time.RFC3339),
		"timezone_info", timezoneInfo)

	// Check if ready for next processing (20-minute interval check)
	if statsBefore.TotalSubscriptions == 0 {
		h.logger.Warn("No subscriptions loaded for rotation")
		return fmt.Errorf("no subscriptions available for rotation")
	}

	if statsBefore.RemainingToday == 0 {
		h.logger.Info("All subscriptions processed for today",
			"processed_today", statsBefore.ProcessedToday,
			"total_subscriptions", statsBefore.TotalSubscriptions,
			"next_reset", statsBefore.EstimatedCompletionTime.Format("15:04:05"))

		// Create successful result even when all done for today
		result.Details = map[string]interface{}{
			"status":              "all_subscriptions_completed_today",
			"processed_today":     statsBefore.ProcessedToday,
			"total_subscriptions": statsBefore.TotalSubscriptions,
		}
		return nil
	}

	// *** CRITICAL FIX: Actually call the processing method ***
	h.logger.Info("Executing next subscription rotation processing")
	
	// Execute the actual rotation processing
	err := h.articleFetchService.ProcessNextSubscriptionRotation(ctx)
	if err != nil {
		h.logger.Error("Failed to process subscription rotation", "error", err)
		return fmt.Errorf("rotation processing failed: %w", err)
	}

	// Get rotation statistics after actual processing
	statsAfter := h.articleFetchService.GetRotationStats()

	// Update result with processing details
	result.Details = map[string]interface{}{
		"processed_today":      statsAfter.ProcessedToday,
		"remaining_today":      statsAfter.RemainingToday,
		"total_subscriptions":  statsAfter.TotalSubscriptions,
		"next_processing_time": statsAfter.NextProcessingTime.Format("15:04:05"),
		"estimated_completion": statsAfter.EstimatedCompletionTime.Format("15:04:05"),
		"current_index":        statsAfter.CurrentIndex,
	}

	h.logger.Info("Rotation processing completed successfully",
		"processed_today", statsAfter.ProcessedToday,
		"remaining_today", statsAfter.RemainingToday,
		"next_processing", statsAfter.NextProcessingTime.Format("15:04:05"))

	return nil
}

// GetStatus returns current scheduling status
func (h *ScheduleHandler) GetStatus() *ScheduleStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Return a copy to prevent race conditions
	statusCopy := *h.status
	return &statusCopy
}

// UpdateConfig updates the scheduling configuration
func (h *ScheduleHandler) UpdateConfig(newConfig *ScheduleConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Validate configuration
	if newConfig.SubscriptionSyncInterval < time.Minute {
		return fmt.Errorf("subscription sync interval too short: minimum 1 minute")
	}
	if newConfig.ArticleFetchInterval < time.Minute {
		return fmt.Errorf("article fetch interval too short: minimum 1 minute")
	}

	oldConfig := h.config
	h.config = newConfig

	// Update status
	h.status.SubscriptionSyncEnabled = newConfig.EnableSubscriptionSync
	h.status.ArticleFetchEnabled = newConfig.EnableArticleFetch

	// Update tickers if intervals changed
	if h.subscriptionTicker != nil && oldConfig.SubscriptionSyncInterval != newConfig.SubscriptionSyncInterval {
		h.subscriptionTicker.Reset(newConfig.SubscriptionSyncInterval)
		h.status.NextSubscriptionSync = time.Now().Add(newConfig.SubscriptionSyncInterval)
	}

	if h.articleFetchTicker != nil && oldConfig.ArticleFetchInterval != newConfig.ArticleFetchInterval {
		h.articleFetchTicker.Reset(newConfig.ArticleFetchInterval)
		h.status.NextArticleFetch = time.Now().Add(newConfig.ArticleFetchInterval)
	}

	h.logger.Info("Schedule configuration updated",
		"subscription_sync_interval", newConfig.SubscriptionSyncInterval,
		"article_fetch_interval", newConfig.ArticleFetchInterval,
		"subscription_sync_enabled", newConfig.EnableSubscriptionSync,
		"article_fetch_enabled", newConfig.EnableArticleFetch)

	return nil
}

// TriggerSubscriptionSync triggers an immediate subscription sync
func (h *ScheduleHandler) TriggerSubscriptionSync() error {
	h.mu.RLock()
	if h.status.SubscriptionSyncRunning {
		h.mu.RUnlock()
		return fmt.Errorf("subscription sync already running")
	}
	h.mu.RUnlock()

	h.logger.Info("Manual subscription sync triggered")
	go h.executeSubscriptionSync()
	return nil
}

// TriggerArticleFetch triggers an immediate article fetch
func (h *ScheduleHandler) TriggerArticleFetch() error {
	h.mu.RLock()
	if h.status.ArticleFetchRunning {
		h.mu.RUnlock()
		return fmt.Errorf("article fetch already running")
	}
	h.mu.RUnlock()

	h.logger.Info("Manual article fetch triggered")
	go h.executeArticleFetch()
	return nil
}

// AddJobResultCallback adds a callback for job results
func (h *ScheduleHandler) AddJobResultCallback(callback func(*JobResult)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.jobResultCallbacks = append(h.jobResultCallbacks, callback)
}

// notifyJobResult notifies all callbacks of job result
func (h *ScheduleHandler) notifyJobResult(result *JobResult) {
	h.mu.RLock()
	callbacks := make([]func(*JobResult), len(h.jobResultCallbacks))
	copy(callbacks, h.jobResultCallbacks)
	h.mu.RUnlock()

	for _, callback := range callbacks {
		go callback(result) // Execute callbacks asynchronously
	}
}

// GetConfig returns current scheduling configuration
func (h *ScheduleHandler) GetConfig() *ScheduleConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Return a copy
	configCopy := *h.config
	return &configCopy
}

// IsRunning returns whether the scheduler is currently running
func (h *ScheduleHandler) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.ctx != nil && h.ctx.Err() == nil
}
