// ABOUTME: This file defines domain models for API synchronization state management
// ABOUTME: Handles continuation tokens and API usage tracking for rate limiting

package models

import (
	"time"

	"github.com/google/uuid"
)

// SyncState represents the synchronization state for continuation tokens
type SyncState struct {
	ID                uuid.UUID `json:"id" db:"id"`
	StreamID          string    `json:"stream_id" db:"stream_id"`
	ContinuationToken string    `json:"continuation_token" db:"continuation_token"`
	LastSync          time.Time `json:"last_sync" db:"last_sync"`
}

// APIUsageTracking represents daily API usage tracking for rate limiting
type APIUsageTracking struct {
	ID                uuid.UUID              `json:"id" db:"id"`
	Date              time.Time              `json:"date" db:"date"`
	Zone1Requests     int                    `json:"zone1_requests" db:"zone1_requests"`
	Zone2Requests     int                    `json:"zone2_requests" db:"zone2_requests"`
	LastReset         time.Time              `json:"last_reset" db:"last_reset"`
	RateLimitHeaders  map[string]interface{} `json:"rate_limit_headers" db:"rate_limit_headers"`
}

// APIUsageInfo represents current API usage information for logging
type APIUsageInfo struct {
	Zone1Requests int `json:"zone1_requests"`
	DailyLimit    int `json:"daily_limit"`
	Remaining     int `json:"remaining"`
}

// APIRateLimitInfo represents comprehensive API rate limit information
type APIRateLimitInfo struct {
	Zone1Usage     int       `json:"zone1_usage"`
	Zone1Limit     int       `json:"zone1_limit"`
	Zone1Remaining int       `json:"zone1_remaining"`
	Zone2Usage     int       `json:"zone2_usage"`
	Zone2Limit     int       `json:"zone2_limit"`
	Zone2Remaining int       `json:"zone2_remaining"`
	ResetTime      time.Time `json:"reset_time"`
	LastUpdated    time.Time `json:"last_updated"`
}

// NewSyncState creates a new sync state for a stream
func NewSyncState(streamID, continuationToken string) *SyncState {
	return &SyncState{
		ID:                uuid.New(),
		StreamID:          streamID,
		ContinuationToken: continuationToken,
		LastSync:          time.Now(),
	}
}

// UpdateContinuationToken updates the continuation token and sync time
func (s *SyncState) UpdateContinuationToken(token string) {
	s.ContinuationToken = token
	s.LastSync = time.Now()
}

// NewAPIUsageTracking creates a new API usage tracking record for today
func NewAPIUsageTracking() *APIUsageTracking {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	return &APIUsageTracking{
		ID:                uuid.New(),
		Date:              today,
		Zone1Requests:     0,
		Zone2Requests:     0,
		LastReset:         now,
		RateLimitHeaders:  make(map[string]interface{}),
	}
}

// IncrementZone1Usage increments Zone 1 request count
func (u *APIUsageTracking) IncrementZone1Usage() {
	u.Zone1Requests++
}

// IncrementZone2Usage increments Zone 2 request count
func (u *APIUsageTracking) IncrementZone2Usage() {
	u.Zone2Requests++
}

// UpdateRateLimitHeaders updates the rate limit headers from API response
func (u *APIUsageTracking) UpdateRateLimitHeaders(headers map[string]interface{}) {
	u.RateLimitHeaders = headers
	u.LastReset = time.Now()
}

// GetUsageInfo returns current usage information for logging
func (u *APIUsageTracking) GetUsageInfo() *APIUsageInfo {
	const dailyLimit = 100 // Zone 1 daily limit
	
	return &APIUsageInfo{
		Zone1Requests: u.Zone1Requests,
		DailyLimit:    dailyLimit,
		Remaining:     dailyLimit - u.Zone1Requests,
	}
}

// ExceedsLimit checks if usage exceeds the daily limit
func (u *APIUsageTracking) ExceedsLimit() bool {
	const dailyLimit = 100 // Zone 1 daily limit
	return u.Zone1Requests >= dailyLimit
}

// ShouldResetUsage checks if usage should be reset (new day)
func (u *APIUsageTracking) ShouldResetUsage() bool {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	return u.Date.Before(today)
}

// ResetUsage resets usage counters for new day
func (u *APIUsageTracking) ResetUsage() {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	u.Date = today
	u.Zone1Requests = 0
	u.Zone2Requests = 0
	u.LastReset = now
	u.RateLimitHeaders = make(map[string]interface{})
}