// ABOUTME: Rate limit manager for Inoreader API usage monitoring and control
// ABOUTME: Handles API quotas, usage tracking, and safety limits to prevent API blocking

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"pre-processor-sidecar/models"
)

// Note: APIUsageRepository interface is defined in inoreader_service.go

// RateLimitConfig represents rate limiting configuration
type RateLimitConfig struct {
	Zone1DailyLimit       int           `json:"zone1_daily_limit"`       // Zone 1 daily limit (read operations)
	Zone2DailyLimit       int           `json:"zone2_daily_limit"`       // Zone 2 daily limit (write operations)
	SafetyBufferPercent   int           `json:"safety_buffer_percent"`   // Safety buffer percentage
	UsageCheckInterval    time.Duration `json:"usage_check_interval"`    // How often to check usage
	HeaderRefreshInterval time.Duration `json:"header_refresh_interval"` // How often to refresh from headers
	AlertThresholds       []int         `json:"alert_thresholds"`        // Alert at these percentages
}

// RateLimitStatus represents current rate limiting status
type RateLimitStatus struct {
	Zone1Usage         int                    `json:"zone1_usage"`
	Zone1Limit         int                    `json:"zone1_limit"`
	Zone1Remaining     int                    `json:"zone1_remaining"`
	Zone2Usage         int                    `json:"zone2_usage"`
	Zone2Limit         int                    `json:"zone2_limit"`
	Zone2Remaining     int                    `json:"zone2_remaining"`
	SafetyBufferActive bool                   `json:"safety_buffer_active"`
	DailyResetTime     time.Time              `json:"daily_reset_time"`
	LastUpdated        time.Time              `json:"last_updated"`
	IsBlocked          bool                   `json:"is_blocked"`
	BlockedReason      string                 `json:"blocked_reason,omitempty"`
	Headers            map[string]interface{} `json:"headers,omitempty"`
}

// RateLimitAlert represents a rate limit alert
type RateLimitAlert struct {
	AlertType    string    `json:"alert_type"` // "warning", "critical", "blocked"
	Message      string    `json:"message"`
	Threshold    int       `json:"threshold"` // Percentage threshold that triggered alert
	CurrentUsage int       `json:"current_usage"`
	DailyLimit   int       `json:"daily_limit"`
	Zone         int       `json:"zone"` // 1 or 2
	Timestamp    time.Time `json:"timestamp"`
}

// RateLimitManager manages API rate limiting and usage monitoring
type RateLimitManager struct {
	config         *RateLimitConfig
	apiUsageRepo   APIUsageRepository
	logger         *slog.Logger
	currentStatus  *RateLimitStatus
	lastUsageCheck time.Time
	alertCallbacks []func(*RateLimitAlert)
	mu             sync.RWMutex
}

// NewRateLimitManager creates a new rate limit manager
func NewRateLimitManager(
	apiUsageRepo APIUsageRepository,
	logger *slog.Logger,
) *RateLimitManager {
	if logger == nil {
		logger = slog.Default()
	}

	// Default configuration based on Inoreader API limits
	config := &RateLimitConfig{
		Zone1DailyLimit:       100,               // Read operations daily limit
		Zone2DailyLimit:       100,               // Write operations daily limit
		SafetyBufferPercent:   10,                // 10% safety buffer
		UsageCheckInterval:    15 * time.Minute,  // Check every 15 minutes
		HeaderRefreshInterval: 5 * time.Minute,   // Refresh from headers every 5 minutes
		AlertThresholds:       []int{50, 75, 90}, // Alert at 50%, 75%, 90%
	}

	return &RateLimitManager{
		config:       config,
		apiUsageRepo: apiUsageRepo,
		logger:       logger,
		currentStatus: &RateLimitStatus{
			Zone1Limit:     config.Zone1DailyLimit,
			Zone2Limit:     config.Zone2DailyLimit,
			DailyResetTime: getNextMidnight(),
			LastUpdated:    time.Now(),
			Headers:        make(map[string]interface{}),
		},
		alertCallbacks: make([]func(*RateLimitAlert), 0),
	}
}

// UpdateFromHeaders updates rate limit status from API response headers
func (r *RateLimitManager) UpdateFromHeaders(ctx context.Context, headers map[string]string, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Parse Inoreader-specific rate limit headers
	if zone1Usage, ok := headers["X-Reader-Zone1-Usage"]; ok {
		if parsed, err := strconv.ParseInt(zone1Usage, 10, 32); err == nil {
			r.currentStatus.Zone1Usage = int(parsed)
		}
	}

	if zone1Limit, ok := headers["X-Reader-Zone1-Limit"]; ok {
		if parsed, err := strconv.ParseInt(zone1Limit, 10, 32); err == nil {
			r.currentStatus.Zone1Limit = int(parsed)
		}
	}

	if zone1Remaining, ok := headers["X-Reader-Zone1-Remaining"]; ok {
		if parsed, err := strconv.ParseInt(zone1Remaining, 10, 32); err == nil {
			r.currentStatus.Zone1Remaining = int(parsed)
		}
	}

	if zone2Usage, ok := headers["X-Reader-Zone2-Usage"]; ok {
		if parsed, err := strconv.ParseInt(zone2Usage, 10, 32); err == nil {
			r.currentStatus.Zone2Usage = int(parsed)
		}
	}

	if zone2Limit, ok := headers["X-Reader-Zone2-Limit"]; ok {
		if parsed, err := strconv.ParseInt(zone2Limit, 10, 32); err == nil {
			r.currentStatus.Zone2Limit = int(parsed)
		}
	}

	if zone2Remaining, ok := headers["X-Reader-Zone2-Remaining"]; ok {
		if parsed, err := strconv.ParseInt(zone2Remaining, 10, 32); err == nil {
			r.currentStatus.Zone2Remaining = int(parsed)
		}
	}

	// Store all headers for debugging
	r.currentStatus.Headers = make(map[string]interface{})
	for key, value := range headers {
		r.currentStatus.Headers[key] = value
	}

	r.currentStatus.LastUpdated = time.Now()

	// Update blocked status
	r.updateBlockedStatus()

	// Check for alerts
	r.checkAndTriggerAlerts()

	// Persist to database
	if err := r.persistUsage(ctx, endpoint); err != nil {
		r.logger.Error("Failed to persist API usage", "error", err)
	}

	r.logger.Debug("Rate limit status updated from headers",
		"zone1_usage", r.currentStatus.Zone1Usage,
		"zone1_limit", r.currentStatus.Zone1Limit,
		"zone2_usage", r.currentStatus.Zone2Usage,
		"zone2_limit", r.currentStatus.Zone2Limit,
		"endpoint", endpoint)

	return nil
}

// CheckAllowed checks if a request is allowed based on current rate limits
func (r *RateLimitManager) CheckAllowed(endpoint string) (allowed bool, reason string, remaining int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if blocked
	if r.currentStatus.IsBlocked {
		return false, r.currentStatus.BlockedReason, 0
	}

	// Determine which zone this endpoint belongs to
	isZone1 := r.isReadOnlyEndpoint(endpoint)

	var usage, limit int
	if isZone1 {
		usage = r.currentStatus.Zone1Usage
		limit = r.currentStatus.Zone1Limit
	} else {
		usage = r.currentStatus.Zone2Usage
		limit = r.currentStatus.Zone2Limit
	}

	// Apply safety buffer
	safetyBuffer := (limit * r.config.SafetyBufferPercent) / 100
	effectiveLimit := limit - safetyBuffer
	remaining = effectiveLimit - usage

	if remaining <= 0 {
		zone := "Zone 1"
		if !isZone1 {
			zone = "Zone 2"
		}
		return false, fmt.Sprintf("%s rate limit exceeded with safety buffer", zone), 0
	}

	return true, "", remaining
}

// GetStatus returns current rate limit status
func (r *RateLimitManager) GetStatus() *RateLimitStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent race conditions
	statusCopy := *r.currentStatus
	statusCopy.Headers = make(map[string]interface{})
	for k, v := range r.currentStatus.Headers {
		statusCopy.Headers[k] = v
	}

	return &statusCopy
}

// GetUsagePercentage returns usage percentage for specified zone
func (r *RateLimitManager) GetUsagePercentage(zone int) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var usage, limit int
	if zone == 1 {
		usage = r.currentStatus.Zone1Usage
		limit = r.currentStatus.Zone1Limit
	} else {
		usage = r.currentStatus.Zone2Usage
		limit = r.currentStatus.Zone2Limit
	}

	if limit == 0 {
		return 0.0
	}

	return (float64(usage) / float64(limit)) * 100.0
}

// AddAlertCallback adds a callback function for rate limit alerts
func (r *RateLimitManager) AddAlertCallback(callback func(*RateLimitAlert)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.alertCallbacks = append(r.alertCallbacks, callback)
}

// updateBlockedStatus updates the blocked status based on current usage
func (r *RateLimitManager) updateBlockedStatus() {
	// Check Zone 1
	zone1Percentage := r.GetUsagePercentage(1)
	zone2Percentage := r.GetUsagePercentage(2)

	safetyThreshold := float64(100 - r.config.SafetyBufferPercent)

	if zone1Percentage >= safetyThreshold {
		r.currentStatus.IsBlocked = true
		r.currentStatus.BlockedReason = fmt.Sprintf("Zone 1 usage exceeded safety threshold: %.1f%%", zone1Percentage)
		r.currentStatus.SafetyBufferActive = true
	} else if zone2Percentage >= safetyThreshold {
		r.currentStatus.IsBlocked = true
		r.currentStatus.BlockedReason = fmt.Sprintf("Zone 2 usage exceeded safety threshold: %.1f%%", zone2Percentage)
		r.currentStatus.SafetyBufferActive = true
	} else {
		r.currentStatus.IsBlocked = false
		r.currentStatus.BlockedReason = ""
		r.currentStatus.SafetyBufferActive = false
	}
}

// checkAndTriggerAlerts checks for threshold violations and triggers alerts
func (r *RateLimitManager) checkAndTriggerAlerts() {
	zone1Percentage := r.GetUsagePercentage(1)
	zone2Percentage := r.GetUsagePercentage(2)

	// Check Zone 1 alerts
	for _, threshold := range r.config.AlertThresholds {
		if zone1Percentage >= float64(threshold) {
			r.triggerAlert(&RateLimitAlert{
				AlertType:    r.getAlertType(threshold),
				Message:      fmt.Sprintf("Zone 1 API usage reached %d%% threshold", threshold),
				Threshold:    threshold,
				CurrentUsage: r.currentStatus.Zone1Usage,
				DailyLimit:   r.currentStatus.Zone1Limit,
				Zone:         1,
				Timestamp:    time.Now(),
			})
		}
	}

	// Check Zone 2 alerts
	for _, threshold := range r.config.AlertThresholds {
		if zone2Percentage >= float64(threshold) {
			r.triggerAlert(&RateLimitAlert{
				AlertType:    r.getAlertType(threshold),
				Message:      fmt.Sprintf("Zone 2 API usage reached %d%% threshold", threshold),
				Threshold:    threshold,
				CurrentUsage: r.currentStatus.Zone2Usage,
				DailyLimit:   r.currentStatus.Zone2Limit,
				Zone:         2,
				Timestamp:    time.Now(),
			})
		}
	}
}

// triggerAlert triggers an alert by calling all registered callbacks
func (r *RateLimitManager) triggerAlert(alert *RateLimitAlert) {
	r.logger.Warn("Rate limit alert triggered",
		"alert_type", alert.AlertType,
		"message", alert.Message,
		"threshold", alert.Threshold,
		"zone", alert.Zone)

	for _, callback := range r.alertCallbacks {
		go callback(alert) // Execute callbacks asynchronously
	}
}

// getAlertType determines alert type based on threshold
func (r *RateLimitManager) getAlertType(threshold int) string {
	if threshold >= 90 {
		return "critical"
	} else if threshold >= 75 {
		return "warning"
	}
	return "info"
}

// isReadOnlyEndpoint determines if an endpoint is read-only (Zone 1)
func (r *RateLimitManager) isReadOnlyEndpoint(endpoint string) bool {
	readOnlyEndpoints := []string{
		"/subscription/list",
		"/stream/contents/",
		"/stream/items/contents",
		"/user-info",
		"/unread-count",
	}

	for _, readOnly := range readOnlyEndpoints {
		if endpoint == readOnly || (readOnly[len(readOnly)-1] == '/' &&
			len(endpoint) > len(readOnly) &&
			endpoint[:len(readOnly)] == readOnly) {
			return true
		}
	}

	return false
}

// persistUsage persists current usage to database
func (r *RateLimitManager) persistUsage(ctx context.Context, endpoint string) error {
	if r.apiUsageRepo == nil {
		return nil // No repository configured
	}

	// Get or create today's usage record
	usage, err := r.apiUsageRepo.GetTodaysUsage(ctx)
	if err != nil {
		// Create new usage record
		usage = models.NewAPIUsageTracking()
		usage.Zone1Requests = r.currentStatus.Zone1Usage
		usage.Zone2Requests = r.currentStatus.Zone2Usage
		usage.UpdateRateLimitHeaders(r.currentStatus.Headers)

		return r.apiUsageRepo.CreateUsageRecord(ctx, usage)
	}

	// Update existing record
	usage.Zone1Requests = r.currentStatus.Zone1Usage
	usage.Zone2Requests = r.currentStatus.Zone2Usage
	usage.UpdateRateLimitHeaders(r.currentStatus.Headers)

	return r.apiUsageRepo.UpdateUsageRecord(ctx, usage)
}

// ResetDailyUsage resets daily usage counters (typically called at midnight)
func (r *RateLimitManager) ResetDailyUsage() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.currentStatus.Zone1Usage = 0
	r.currentStatus.Zone2Usage = 0
	r.currentStatus.Zone1Remaining = r.currentStatus.Zone1Limit
	r.currentStatus.Zone2Remaining = r.currentStatus.Zone2Limit
	r.currentStatus.DailyResetTime = getNextMidnight()
	r.currentStatus.IsBlocked = false
	r.currentStatus.BlockedReason = ""
	r.currentStatus.SafetyBufferActive = false
	r.currentStatus.LastUpdated = time.Now()

	r.logger.Info("Daily usage counters reset",
		"next_reset", r.currentStatus.DailyResetTime)
}

// GetUsageStats returns usage statistics for monitoring
func (r *RateLimitManager) GetUsageStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"zone1_usage_percent":  r.GetUsagePercentage(1),
		"zone2_usage_percent":  r.GetUsagePercentage(2),
		"safety_buffer_active": r.currentStatus.SafetyBufferActive,
		"is_blocked":           r.currentStatus.IsBlocked,
		"blocked_reason":       r.currentStatus.BlockedReason,
		"daily_reset_time":     r.currentStatus.DailyResetTime,
		"last_updated":         r.currentStatus.LastUpdated,
		"zone1_remaining":      r.currentStatus.Zone1Remaining,
		"zone2_remaining":      r.currentStatus.Zone2Remaining,
	}
}

// UpdateConfig updates rate limit configuration
func (r *RateLimitManager) UpdateConfig(newConfig *RateLimitConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = newConfig
	r.logger.Info("Rate limit configuration updated",
		"zone1_limit", newConfig.Zone1DailyLimit,
		"zone2_limit", newConfig.Zone2DailyLimit,
		"safety_buffer", newConfig.SafetyBufferPercent)
}

// getNextMidnight returns the next midnight timestamp
func getNextMidnight() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}
