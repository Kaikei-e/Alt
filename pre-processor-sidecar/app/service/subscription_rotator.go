// ABOUTME: SubscriptionRotator handles intelligent rotation of subscription processing
// ABOUTME: Ensures all 40 subscriptions are processed once daily with 20-minute intervals

package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SubscriptionRotator manages round-robin processing of subscriptions
type SubscriptionRotator struct {
	subscriptions    []uuid.UUID
	currentIndex     int
	lastProcessed    map[uuid.UUID]time.Time
	intervalMinutes  int    // 20分間隔
	maxDaily         int    // 40サブスクリプション/日
	mu               sync.RWMutex
	logger           *slog.Logger
	lastResetDate    time.Time
	randomStartEnabled bool  // Enable random starting position
	startingIndex     int    // Random starting index for rotation
	timezone          *time.Location // タイムゾーン設定を追加
}

// RotationStats provides statistics about rotation processing
type RotationStats struct {
	TotalSubscriptions      int       `json:"total_subscriptions"`
	ProcessedToday         int       `json:"processed_today"`
	RemainingToday         int       `json:"remaining_today"`
	CurrentIndex           int       `json:"current_index"`
	LastProcessedTime      time.Time `json:"last_processed_time"`
	NextProcessingTime     time.Time `json:"next_processing_time"`
	EstimatedCompletionTime time.Time `json:"estimated_completion_time"`
}

// NewSubscriptionRotator creates a new subscription rotator
func NewSubscriptionRotator(logger *slog.Logger) *SubscriptionRotator {
	if logger == nil {
		logger = slog.Default()
	}

	// タイムゾーンを設定（JST優先、環境変数でオーバーライド可能）
	timezone := time.UTC // デフォルトはUTC
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			timezone = loc
		}
	} else {
		// JST (Asia/Tokyo) をデフォルトに設定
		if loc, err := time.LoadLocation("Asia/Tokyo"); err == nil {
			timezone = loc
		}
	}

	// 最大日次処理回数を環境変数から取得（デフォルトは1回/日）
	maxDailyRotations := 1
	if env := os.Getenv("MAX_DAILY_ROTATIONS"); env != "" {
		if val, err := strconv.Atoi(env); err == nil && val > 0 && val <= 1000 {
			maxDailyRotations = val
		} else {
			logger.Warn("Invalid MAX_DAILY_ROTATIONS value, using default",
				"provided", env,
				"default", maxDailyRotations)
		}
	}

	now := time.Now().In(timezone)
	
	// 正しい0時に切り捨て（year, month, dayのみを使用）
	year, month, day := now.Date()
	todayInTimezone := time.Date(year, month, day, 0, 0, 0, 0, timezone)
	
	logger.Info("Initializing SubscriptionRotator",
		"max_daily_rotations", maxDailyRotations,
		"interval_minutes", 20,
		"timezone", timezone.String())
	
	return &SubscriptionRotator{
		subscriptions:      make([]uuid.UUID, 0),
		lastProcessed:      make(map[uuid.UUID]time.Time),
		intervalMinutes:    20,   // 20分間隔（API制限対応）46 × 20min = 15.3h で1サイクル
		maxDaily:           maxDailyRotations, // 環境変数で制御可能（デフォルト1回/日）
		currentIndex:       0,
		logger:            logger,
		lastResetDate:     todayInTimezone, // タイムゾーン対応
		randomStartEnabled: false, // Default: maintain existing behavior
		startingIndex:      0,
		timezone:          timezone,
	}
}

// LoadSubscriptions loads all available subscriptions into rotator
func (sr *SubscriptionRotator) LoadSubscriptions(ctx context.Context, subscriptions []uuid.UUID) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if len(subscriptions) == 0 {
		return fmt.Errorf("no subscriptions provided")
	}

	sr.subscriptions = make([]uuid.UUID, len(subscriptions))
	copy(sr.subscriptions, subscriptions)

	// Shuffle subscriptions for fair distribution
	sr.shuffleSubscriptions()

	// Set random starting index if enabled
	if sr.randomStartEnabled {
		sr.generateRandomStartingIndex()
		sr.currentIndex = sr.startingIndex
	}

	sr.logger.Info("Loaded subscriptions into rotator",
		"total_subscriptions", len(sr.subscriptions),
		"interval_minutes", sr.intervalMinutes,
		"estimated_completion_hours", float64(len(sr.subscriptions)*sr.intervalMinutes)/60.0,
		"random_start_enabled", sr.randomStartEnabled,
		"starting_index", sr.currentIndex)

	return nil
}

// GetNextSubscription returns the next subscription to process
func (sr *SubscriptionRotator) GetNextSubscription() (uuid.UUID, bool) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if len(sr.subscriptions) == 0 {
		sr.logger.Warn("No subscriptions available for processing")
		return uuid.Nil, false
	}

	// Check if daily reset is needed
	now := time.Now()
	if sr.shouldResetDaily(now) {
		sr.resetDailyRotation(now)
	}

	// Check if all subscriptions have been processed today
	if sr.hasCompletedDailyRotation() {
		sr.logger.Info("All subscriptions processed for today",
			"processed_count", len(sr.subscriptions),
			"next_reset", sr.getNextResetTime())
		return uuid.Nil, false
	}

	// Get next subscription in rotation (with circular support)
	actualIndex := sr.currentIndex % len(sr.subscriptions)
	targetSub := sr.subscriptions[actualIndex]
	sr.lastProcessed[targetSub] = now
	sr.currentIndex++

	sr.logger.Debug("Selected subscription for processing",
		"subscription_id", targetSub,
		"index", sr.currentIndex-1,
		"total", len(sr.subscriptions),
		"remaining_today", len(sr.subscriptions)-sr.currentIndex)

	return targetSub, true
}

// shouldResetDaily checks if daily rotation reset is needed
func (sr *SubscriptionRotator) shouldResetDaily(now time.Time) bool {
	// タイムゾーンを考慮した日付比較
	nowInTimezone := now.In(sr.timezone)
	
	// 正しい0時に切り捨て（year, month, dayのみを使用）
	year, month, day := nowInTimezone.Date()
	todayInTimezone := time.Date(year, month, day, 0, 0, 0, 0, sr.timezone)
	
	sr.logger.Debug("Daily reset check",
		"current_time_utc", now.Format(time.RFC3339),
		"current_time_local", nowInTimezone.Format(time.RFC3339),
		"today_local", todayInTimezone.Format(time.RFC3339),
		"last_reset_date", sr.lastResetDate.Format(time.RFC3339),
		"timezone", sr.timezone.String())
	
	return !sr.lastResetDate.Equal(todayInTimezone)
}

// resetDailyRotation resets the rotation for a new day
func (sr *SubscriptionRotator) resetDailyRotation(now time.Time) {
	// タイムゾーンを考慮した日付処理
	nowInTimezone := now.In(sr.timezone)
	
	// 正しい0時に切り捨て（year, month, dayのみを使用）
	year, month, day := nowInTimezone.Date()
	todayInTimezone := time.Date(year, month, day, 0, 0, 0, 0, sr.timezone)
	
	sr.logger.Info("Resetting daily rotation",
		"previous_date", sr.lastResetDate.Format("2006-01-02"),
		"new_date_utc", now.Format("2006-01-02"),
		"new_date_local", nowInTimezone.Format("2006-01-02"),
		"processed_yesterday", sr.currentIndex,
		"timezone", sr.timezone.String())

	sr.lastProcessed = make(map[uuid.UUID]time.Time)
	sr.lastResetDate = todayInTimezone // タイムゾーン対応

	// Shuffle subscriptions for better distribution
	sr.shuffleSubscriptions()

	// Reset to starting position (random or 0)
	if sr.randomStartEnabled {
		sr.generateRandomStartingIndex()
		sr.currentIndex = sr.startingIndex
	} else {
		sr.currentIndex = 0
	}

	sr.logger.Info("Daily rotation reset completed",
		"total_subscriptions", len(sr.subscriptions),
		"reset_to_index", sr.currentIndex,
		"estimated_completion", sr.getEstimatedCompletionTime().Format("15:04:05"))
}

// shuffleSubscriptions randomizes the order of subscriptions
func (sr *SubscriptionRotator) shuffleSubscriptions() {
	if len(sr.subscriptions) <= 1 {
		return
	}

	rand.Shuffle(len(sr.subscriptions), func(i, j int) {
		sr.subscriptions[i], sr.subscriptions[j] = sr.subscriptions[j], sr.subscriptions[i]
	})

	sr.logger.Debug("Shuffled subscription order for fair distribution",
		"total_subscriptions", len(sr.subscriptions))
}

// GetStats returns current rotation statistics
func (sr *SubscriptionRotator) GetStats() RotationStats {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	maxProcessingToday := len(sr.subscriptions) * sr.maxDaily
	remaining := maxProcessingToday - sr.currentIndex
	if remaining < 0 {
		remaining = 0
	}

	var lastProcessedTime time.Time
	for _, processedTime := range sr.lastProcessed {
		if processedTime.After(lastProcessedTime) {
			lastProcessedTime = processedTime
		}
	}

	nextProcessingTime := lastProcessedTime.Add(time.Duration(sr.intervalMinutes) * time.Minute)
	estimatedCompletion := sr.getEstimatedCompletionTime()

	return RotationStats{
		TotalSubscriptions:      len(sr.subscriptions),
		ProcessedToday:         sr.currentIndex,
		RemainingToday:         remaining,
		CurrentIndex:           sr.currentIndex,
		LastProcessedTime:      lastProcessedTime,
		NextProcessingTime:     nextProcessingTime,
		EstimatedCompletionTime: estimatedCompletion,
	}
}

// getEstimatedCompletionTime calculates when all subscriptions will be processed
func (sr *SubscriptionRotator) getEstimatedCompletionTime() time.Time {
	maxProcessingToday := len(sr.subscriptions) * sr.maxDaily
	if sr.currentIndex >= maxProcessingToday {
		// All done for today
		return sr.getNextResetTime()
	}

	remaining := maxProcessingToday - sr.currentIndex
	estimatedMinutes := remaining * sr.intervalMinutes

	return time.Now().Add(time.Duration(estimatedMinutes) * time.Minute)
}

// getNextResetTime returns the next daily reset time (midnight in local timezone)
func (sr *SubscriptionRotator) getNextResetTime() time.Time {
	nowInTimezone := time.Now().In(sr.timezone)
	
	// 明日の0時を正確に計算
	year, month, day := nowInTimezone.Date()
	tomorrow := time.Date(year, month, day+1, 0, 0, 0, 0, sr.timezone)
	
	return tomorrow
}

// IsReadyForNext checks if enough time has passed for next processing
func (sr *SubscriptionRotator) IsReadyForNext() bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if len(sr.lastProcessed) == 0 {
		// No previous processing, ready to start
		return true
	}

	// Find the most recent processing time
	var lastTime time.Time
	for _, processedTime := range sr.lastProcessed {
		if processedTime.After(lastTime) {
			lastTime = processedTime
		}
	}

	// Check if interval has passed
	nextAllowedTime := lastTime.Add(time.Duration(sr.intervalMinutes) * time.Minute)
	return time.Now().After(nextAllowedTime)
}

// GetInterval returns the current processing interval in minutes
func (sr *SubscriptionRotator) GetInterval() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.intervalMinutes
}

// SetInterval updates the processing interval
func (sr *SubscriptionRotator) SetInterval(minutes int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if minutes < 1 {
		minutes = 1
	}
	if minutes > 240 { // Max 4 hours
		minutes = 240
	}

	oldInterval := sr.intervalMinutes
	sr.intervalMinutes = minutes

	sr.logger.Info("Updated rotation interval",
		"old_interval_minutes", oldInterval,
		"new_interval_minutes", sr.intervalMinutes,
		"estimated_completion_hours", float64(len(sr.subscriptions)*sr.intervalMinutes)/60.0)
}

// GetProcessingStatus returns human-readable processing status
func (sr *SubscriptionRotator) GetProcessingStatus() string {
	stats := sr.GetStats()
	maxProcessingToday := stats.TotalSubscriptions * sr.maxDaily

	if stats.RemainingToday == 0 {
		return fmt.Sprintf("Completed %d/%d (max daily rotations: %d). Next reset: %s",
			stats.ProcessedToday, maxProcessingToday, sr.maxDaily,
			stats.EstimatedCompletionTime.Format("15:04"))
	}

	return fmt.Sprintf("Processing %d/%d (max daily rotations: %d). Estimated completion: %s",
		stats.ProcessedToday, maxProcessingToday, sr.maxDaily,
		stats.EstimatedCompletionTime.Format("15:04"))
}

// generateRandomStartingIndex generates a random starting index for rotation
func (sr *SubscriptionRotator) generateRandomStartingIndex() {
	if len(sr.subscriptions) == 0 {
		sr.startingIndex = 0
		return
	}
	
	sr.startingIndex = rand.Intn(len(sr.subscriptions))
	sr.logger.Info("Generated random starting index",
		"starting_index", sr.startingIndex,
		"total_subscriptions", len(sr.subscriptions))
}

// hasCompletedDailyRotation checks if all subscriptions have been processed for today
func (sr *SubscriptionRotator) hasCompletedDailyRotation() bool {
	if len(sr.subscriptions) == 0 {
		return true
	}

	// Check if we've completed the configured number of daily rotations
	// maxDaily = 1: each subscription processed once per day (original behavior)
	// maxDaily = 48: each subscription processed 48 times per day (every 30min)
	maxProcessingToday := len(sr.subscriptions) * sr.maxDaily
	
	sr.logger.Debug("Daily rotation check",
		"current_index", sr.currentIndex,
		"subscriptions", len(sr.subscriptions), 
		"max_daily", sr.maxDaily,
		"max_processing_today", maxProcessingToday,
		"completed", sr.currentIndex >= maxProcessingToday)
	
	return sr.currentIndex >= maxProcessingToday
}

// EnableRandomStart enables random starting position for rotation
func (sr *SubscriptionRotator) EnableRandomStart() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	
	sr.randomStartEnabled = true
	sr.logger.Info("Random start enabled",
		"current_subscriptions", len(sr.subscriptions))
}

// DisableRandomStart disables random starting position (default behavior)
func (sr *SubscriptionRotator) DisableRandomStart() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	
	sr.randomStartEnabled = false
	sr.logger.Info("Random start disabled - using sequential rotation")
}

// IsRandomStartEnabled returns current random start status
func (sr *SubscriptionRotator) IsRandomStartEnabled() bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.randomStartEnabled
}

// GetStartingIndex returns the current starting index
func (sr *SubscriptionRotator) GetStartingIndex() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.startingIndex
}

// GetNextSubscriptionBatch returns a batch of subscriptions to process
func (sr *SubscriptionRotator) GetNextSubscriptionBatch(batchSize int) []uuid.UUID {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if len(sr.subscriptions) == 0 {
		sr.logger.Warn("No subscriptions available for batch processing")
		return []uuid.UUID{}
	}

	// Check if daily reset is needed
	now := time.Now()
	if sr.shouldResetDaily(now) {
		sr.resetDailyRotation(now)
	}

	// Check if all subscriptions have been processed today
	if sr.hasCompletedDailyRotation() {
		sr.logger.Info("All subscriptions processed for today",
			"processed_count", len(sr.subscriptions),
			"next_reset", sr.getNextResetTime())
		return []uuid.UUID{}
	}

	batch := make([]uuid.UUID, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		// Check if we have more subscriptions to process
		if sr.currentIndex >= len(sr.subscriptions)*sr.maxDaily {
			break
		}

		actualIndex := sr.currentIndex % len(sr.subscriptions)
		targetSub := sr.subscriptions[actualIndex]
		sr.lastProcessed[targetSub] = now
		sr.currentIndex++

		batch = append(batch, targetSub)

		sr.logger.Debug("Added subscription to batch",
			"subscription_id", targetSub,
			"index", sr.currentIndex-1,
			"batch_position", i+1)
	}

	if len(batch) > 0 {
		sr.logger.Info("Created subscription batch",
			"batch_size", len(batch),
			"processed_today", sr.currentIndex,
			"remaining_today", len(sr.subscriptions)*sr.maxDaily-sr.currentIndex)
	}

	return batch
}

// GetBatchProcessingStatus returns status for batch processing
func (sr *SubscriptionRotator) GetBatchProcessingStatus(batchSize int) string {
	stats := sr.GetStats()
	maxProcessingToday := stats.TotalSubscriptions * sr.maxDaily

	if stats.RemainingToday == 0 {
		return fmt.Sprintf("Batch processing completed %d/%d (batch size: %d). Next reset: %s",
			stats.ProcessedToday, maxProcessingToday, batchSize,
			stats.EstimatedCompletionTime.Format("15:04"))
	}

	remainingBatches := (stats.RemainingToday + batchSize - 1) / batchSize
	estimatedHours := float64(remainingBatches) * 0.5 // 30分間隔

	return fmt.Sprintf("Batch processing %d/%d (batch size: %d). Remaining batches: %d (~%.1fh)",
		stats.ProcessedToday, maxProcessingToday, batchSize,
		remainingBatches, estimatedHours)
}

// GetTimezoneInfo returns current timezone information for debugging
func (sr *SubscriptionRotator) GetTimezoneInfo() map[string]interface{} {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	now := time.Now()
	nowInTimezone := now.In(sr.timezone)
	
	return map[string]interface{}{
		"timezone_name":       sr.timezone.String(),
		"current_time_utc":    now.Format(time.RFC3339),
		"current_time_local":  nowInTimezone.Format(time.RFC3339),
		"last_reset_date":     sr.lastResetDate.Format(time.RFC3339),
		"next_reset_time":     sr.getNextResetTime().Format(time.RFC3339),
		"hours_until_reset":   sr.getNextResetTime().Sub(nowInTimezone).Hours(),
	}
}