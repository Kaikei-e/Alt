// ABOUTME: SubscriptionRotator handles intelligent rotation of subscription processing
// ABOUTME: Ensures all 40 subscriptions are processed once daily with 20-minute intervals

package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
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

	return &SubscriptionRotator{
		subscriptions:   make([]uuid.UUID, 0),
		lastProcessed:   make(map[uuid.UUID]time.Time),
		intervalMinutes: 18,   // 18分間隔 (46 subscriptions × 18min = 13.8 hours, API optimized)
		maxDaily:        46,   // 1日46個処理（全サブスクリプション）
		currentIndex:    0,
		logger:         logger,
		lastResetDate:  time.Now().Truncate(24 * time.Hour),
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

	sr.logger.Info("Loaded subscriptions into rotator",
		"total_subscriptions", len(sr.subscriptions),
		"interval_minutes", sr.intervalMinutes,
		"estimated_completion_hours", float64(len(sr.subscriptions)*sr.intervalMinutes)/60.0)

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
	if sr.currentIndex >= len(sr.subscriptions) {
		sr.logger.Info("All subscriptions processed for today",
			"processed_count", len(sr.subscriptions),
			"next_reset", sr.getNextResetTime())
		return uuid.Nil, false
	}

	// Get next subscription in rotation
	targetSub := sr.subscriptions[sr.currentIndex]
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
	today := now.Truncate(24 * time.Hour)
	return !sr.lastResetDate.Equal(today)
}

// resetDailyRotation resets the rotation for a new day
func (sr *SubscriptionRotator) resetDailyRotation(now time.Time) {
	sr.logger.Info("Resetting daily rotation",
		"previous_date", sr.lastResetDate.Format("2006-01-02"),
		"new_date", now.Format("2006-01-02"),
		"processed_yesterday", sr.currentIndex)

	sr.currentIndex = 0
	sr.lastProcessed = make(map[uuid.UUID]time.Time)
	sr.lastResetDate = now.Truncate(24 * time.Hour)

	// Shuffle subscriptions for better distribution
	sr.shuffleSubscriptions()

	sr.logger.Info("Daily rotation reset completed",
		"total_subscriptions", len(sr.subscriptions),
		"estimated_completion", sr.getEstimatedCompletionTime())
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

	remaining := len(sr.subscriptions) - sr.currentIndex
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
	if sr.currentIndex >= len(sr.subscriptions) {
		// All done for today
		return sr.getNextResetTime()
	}

	remaining := len(sr.subscriptions) - sr.currentIndex
	estimatedMinutes := remaining * sr.intervalMinutes

	return time.Now().Add(time.Duration(estimatedMinutes) * time.Minute)
}

// getNextResetTime returns the next daily reset time (midnight)
func (sr *SubscriptionRotator) getNextResetTime() time.Time {
	tomorrow := time.Now().Add(24 * time.Hour)
	return tomorrow.Truncate(24 * time.Hour)
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

	if stats.RemainingToday == 0 {
		return fmt.Sprintf("Completed %d/%d subscriptions for today. Next reset: %s",
			stats.ProcessedToday, stats.TotalSubscriptions,
			stats.EstimatedCompletionTime.Format("15:04"))
	}

	return fmt.Sprintf("Processing %d/%d subscriptions. Estimated completion: %s",
		stats.ProcessedToday, stats.TotalSubscriptions,
		stats.EstimatedCompletionTime.Format("15:04"))
}