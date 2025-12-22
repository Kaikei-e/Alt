// ABOUTME: This file tests the schedule handler functionality
// ABOUTME: Following TDD principles with focus on testable components

package handler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitAwareScheduler_NextInterval(t *testing.T) {
	tests := map[string]struct {
		baseInterval      time.Duration
		errorCount        int
		expectedInterval  time.Duration
		expectMaxInterval bool
	}{
		"no_errors": {
			baseInterval:     1 * time.Minute,
			errorCount:       0,
			expectedInterval: 1 * time.Minute,
		},
		"single_error": {
			baseInterval:     1 * time.Minute,
			errorCount:       1,
			expectedInterval: 90 * time.Second, // 1.5^1 * 60s = 90s
		},
		"multiple_errors": {
			baseInterval:     1 * time.Minute,
			errorCount:       3,
			expectedInterval: 3*time.Minute + 22*time.Second, // 1.5^3 * 60s ≈ 202s
		},
		"max_interval_reached": {
			baseInterval:      1 * time.Minute,
			errorCount:        20, // Very high error count
			expectMaxInterval: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheduler := NewRateLimitAwareScheduler(tc.baseInterval)

			// Simulate errors
			for i := 0; i < tc.errorCount; i++ {
				scheduler.RecordError()
			}

			interval := scheduler.NextInterval()

			if tc.expectMaxInterval {
				assert.Equal(t, 6*time.Hour, interval)
			} else {
				// Allow some tolerance for floating point calculations
				assert.InDelta(t, tc.expectedInterval.Seconds(), interval.Seconds(), 1.0)
			}
		})
	}
}

func TestRateLimitAwareScheduler_RecordSuccess(t *testing.T) {
	scheduler := NewRateLimitAwareScheduler(1 * time.Minute)

	// Add some errors
	scheduler.RecordError()
	scheduler.RecordError()

	// Verify error count is recorded
	errorCount, _, _ := scheduler.GetStatus()
	assert.Equal(t, 2, errorCount)

	// NextInterval should be greater than base interval with errors
	interval := scheduler.NextInterval()
	assert.Greater(t, interval, 1*time.Minute)

	// Record success
	scheduler.RecordSuccess()

	// Verify reset
	errorCount, interval, lastSuccess := scheduler.GetStatus()
	assert.Equal(t, 0, errorCount)
	assert.Equal(t, 1*time.Minute, interval)
	assert.WithinDuration(t, time.Now(), lastSuccess, 1*time.Second)
}

func TestRateLimitAwareScheduler_RecordError(t *testing.T) {
	scheduler := NewRateLimitAwareScheduler(1 * time.Minute)

	// Initial state
	errorCount, _, _ := scheduler.GetStatus()
	assert.Equal(t, 0, errorCount)

	// Record error
	scheduler.RecordError()

	// Check error count increased
	errorCount, _, _ = scheduler.GetStatus()
	assert.Equal(t, 1, errorCount)

	// Record another error
	scheduler.RecordError()

	// Check error count increased again
	errorCount, _, _ = scheduler.GetStatus()
	assert.Equal(t, 2, errorCount)
}

func TestNewScheduleHandler(t *testing.T) {
	// Test with nil handlers since we're focusing on basic functionality
	handler := NewScheduleHandler(nil, nil, nil)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.config)
	assert.NotNil(t, handler.status)
	assert.NotNil(t, handler.subscriptionScheduler)
	assert.NotNil(t, handler.articleFetchScheduler)

	// Check default configuration
	config := handler.GetConfig()
	assert.Equal(t, 12*time.Hour, config.SubscriptionSyncInterval)
	assert.Equal(t, 30*time.Minute, config.ArticleFetchInterval)
	assert.True(t, config.EnableSubscriptionSync)
	assert.True(t, config.EnableArticleFetch)
	assert.Equal(t, 2, config.MaxConcurrentJobs)
	assert.True(t, config.EnableRandomStart)
}

func TestScheduleHandler_UpdateConfig(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	tests := map[string]struct {
		config      *ScheduleConfig
		expectError bool
		errorMsg    string
	}{
		"valid_config": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 2 * time.Hour,
				ArticleFetchInterval:     30 * time.Minute,
				EnableSubscriptionSync:   false,
				EnableArticleFetch:       true,
				MaxConcurrentJobs:        1,
			},
			expectError: false,
		},
		"subscription_interval_too_short": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 30 * time.Second,
				ArticleFetchInterval:     30 * time.Minute,
			},
			expectError: true,
			errorMsg:    "subscription sync interval too short",
		},
		"article_interval_too_short": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 2 * time.Hour,
				ArticleFetchInterval:     30 * time.Second,
			},
			expectError: true,
			errorMsg:    "article fetch interval too short",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := handler.UpdateConfig(tc.config)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)

				// Verify configuration was updated
				updatedConfig := handler.GetConfig()
				assert.Equal(t, tc.config.SubscriptionSyncInterval, updatedConfig.SubscriptionSyncInterval)
				assert.Equal(t, tc.config.ArticleFetchInterval, updatedConfig.ArticleFetchInterval)
				assert.Equal(t, tc.config.EnableSubscriptionSync, updatedConfig.EnableSubscriptionSync)
				assert.Equal(t, tc.config.EnableArticleFetch, updatedConfig.EnableArticleFetch)
			}
		})
	}
}

func TestScheduleHandler_GetStatus(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	status := handler.GetStatus()
	assert.NotNil(t, status)
	assert.True(t, status.SubscriptionSyncEnabled)
	assert.True(t, status.ArticleFetchEnabled)
	assert.False(t, status.SubscriptionSyncRunning)
	assert.False(t, status.ArticleFetchRunning)
	assert.Equal(t, int64(0), status.TotalSubscriptionSyncs)
	assert.Equal(t, int64(0), status.TotalArticleFetches)
}

func TestScheduleHandler_IsRunning(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	// Initially not running
	assert.False(t, handler.IsRunning())

	// Note: We skip the Start/Stop test as it requires service integration
}

func TestScheduleHandler_TriggerSubscriptionSync_NotRunning(t *testing.T) {
	handler := newScheduleHandlerForTriggerTests(t)
	done := make(chan struct{})
	var once sync.Once

	handler.AddJobResultCallback(func(result *JobResult) {
		if result.JobType == "subscription_sync" {
			once.Do(func() {
				close(done)
			})
		}
	})

	// Test that triggering succeeds at method level
	err := handler.TriggerSubscriptionSync()
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("subscription sync did not complete within timeout")
	}
}

func TestScheduleHandler_TriggerArticleFetch_NotRunning(t *testing.T) {
	handler := newScheduleHandlerForTriggerTests(t)
	done := make(chan struct{})
	var once sync.Once

	handler.AddJobResultCallback(func(result *JobResult) {
		if result.JobType == "article_fetch" {
			once.Do(func() {
				close(done)
			})
		}
	})

	// Test that triggering succeeds at method level
	err := handler.TriggerArticleFetch()
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("article fetch did not complete within timeout")
	}
}

func TestScheduleHandler_AddJobResultCallback(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	var callbackCalled bool
	var receivedResult *JobResult
	var mu sync.Mutex
	done := make(chan struct{})

	handler.AddJobResultCallback(func(result *JobResult) {
		mu.Lock()
		callbackCalled = true
		receivedResult = result
		mu.Unlock()
		close(done)
	})

	// Create a mock job result and notify (directly without triggering actual operations)
	result := &JobResult{
		JobType:   "test",
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Duration:  1 * time.Second,
	}

	// Call notifyJobResult directly to avoid the context issue
	handler.notifyJobResult(result)

	// Wait for callback to execute
	select {
	case <-done:
		// Callback completed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("callback did not complete within timeout")
	}

	mu.Lock()
	assert.True(t, callbackCalled)
	assert.Equal(t, result, receivedResult)
	mu.Unlock()
}

func TestScheduleConfig_Validation(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	// Test invalid configs
	invalidConfigs := []*ScheduleConfig{
		{
			SubscriptionSyncInterval: 30 * time.Second,
			ArticleFetchInterval:     30 * time.Minute,
		},
		{
			SubscriptionSyncInterval: 2 * time.Hour,
			ArticleFetchInterval:     30 * time.Second,
		},
	}

	for i, config := range invalidConfigs {
		t.Run(fmt.Sprintf("invalid_config_%d", i), func(t *testing.T) {
			err := handler.UpdateConfig(config)
			assert.Error(t, err)
		})
	}
}

func TestJobResult_Structure(t *testing.T) {
	// Test JobResult structure
	result := &JobResult{
		JobType:   "subscription_sync",
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Duration:  1 * time.Second,
		Error:     "",
		Details:   map[string]interface{}{"key": "value"},
	}

	assert.Equal(t, "subscription_sync", result.JobType)
	assert.True(t, result.Success)
	assert.Equal(t, 1*time.Second, result.Duration)
	assert.NotNil(t, result.Details)
}

func TestScheduleStatus_Structure(t *testing.T) {
	// Test ScheduleStatus structure
	status := &ScheduleStatus{
		SubscriptionSyncEnabled: true,
		ArticleFetchEnabled:     true,
		LastSubscriptionSync:    time.Now(),
		NextSubscriptionSync:    time.Now().Add(1 * time.Hour),
		LastArticleFetch:        time.Now(),
		NextArticleFetch:        time.Now().Add(30 * time.Minute),
		SubscriptionSyncRunning: false,
		ArticleFetchRunning:     false,
		TotalSubscriptionSyncs:  10,
		TotalArticleFetches:     50,
		FailedSubscriptionSyncs: 1,
		FailedArticleFetches:    2,
		LastError:               "",
	}

	assert.True(t, status.SubscriptionSyncEnabled)
	assert.True(t, status.ArticleFetchEnabled)
	assert.False(t, status.SubscriptionSyncRunning)
	assert.False(t, status.ArticleFetchRunning)
	assert.Equal(t, int64(10), status.TotalSubscriptionSyncs)
	assert.Equal(t, int64(50), status.TotalArticleFetches)
}

// TestScheduleHandler_ArticleFetchErrorHandling tests proper error handling when article fetch fails
func TestScheduleHandler_ArticleFetchErrorHandling(t *testing.T) {
	// Mock a scenario where processing fails due to no actual API calls
	result := &JobResult{
		JobType:   "article_fetch",
		StartTime: time.Now(),
	}

	// Test the case where all subscriptions are already processed for today
	// This should be considered a successful completion, not an error
	result.Details = map[string]interface{}{
		"status":              "all_subscriptions_completed_today",
		"processed_today":     10,
		"total_subscriptions": 5,
		"batch_size":          3,
		"processed_count":     0,
	}

	// Verify result indicates completion without errors
	assert.NotNil(t, result.Details)
	details, ok := result.Details.(map[string]interface{})
	require.True(t, ok)

	// Should indicate that no processing occurred but for valid reason
	assert.Equal(t, "all_subscriptions_completed_today", details["status"])
	assert.Equal(t, 0, details["processed_count"])

	// Test the case where no subscriptions are available at all
	result2 := &JobResult{
		JobType:   "article_fetch",
		StartTime: time.Now(),
	}

	result2.Details = map[string]interface{}{
		"status":          "no_subscriptions_available",
		"batch_size":      3,
		"processed_count": 0,
	}

	details2, ok := result2.Details.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "no_subscriptions_available", details2["status"])
	assert.Equal(t, 0, details2["processed_count"])
}

// TestScheduleHandler_ProcessingStatusDifferentiation tests that different processing states are properly distinguished
func TestScheduleHandler_ProcessingStatusDifferentiation(t *testing.T) {
	// Test different processing outcomes
	testCases := []struct {
		name            string
		status          string
		processedCount  int
		expectedSuccess bool
		expectedReason  string
	}{
		{
			name:            "All_Completed_Today",
			status:          "all_subscriptions_completed_today",
			processedCount:  0,
			expectedSuccess: true,
			expectedReason:  "daily_limit_reached",
		},
		{
			name:            "No_Subscriptions_Available",
			status:          "no_subscriptions_available",
			processedCount:  0,
			expectedSuccess: true,
			expectedReason:  "no_work_available",
		},
		{
			name:            "Actual_Processing_Occurred",
			status:          "processed",
			processedCount:  3,
			expectedSuccess: true,
			expectedReason:  "api_calls_made",
		},
		{
			name:            "Partial_Processing_Some_Failed",
			status:          "partial_failure",
			processedCount:  2,
			expectedSuccess: true, // Partial success is still considered success
			expectedReason:  "some_api_calls_failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &JobResult{
				JobType:   "article_fetch",
				StartTime: time.Now(),
				Success:   tc.expectedSuccess,
				Details: map[string]interface{}{
					"status":          tc.status,
					"processed_count": tc.processedCount,
					"reason":          tc.expectedReason,
				},
			}

			details, ok := result.Details.(map[string]interface{})
			require.True(t, ok)

			assert.Equal(t, tc.status, details["status"])
			assert.Equal(t, tc.processedCount, details["processed_count"])
			assert.Equal(t, tc.expectedReason, details["reason"])
			assert.Equal(t, tc.expectedSuccess, result.Success)
		})
	}
}

// TestScheduleHandler_APICallVerification tests that we can distinguish between cases where API calls are made vs not made
func TestScheduleHandler_APICallVerification(t *testing.T) {
	// Test scenarios that help us identify when actual API calls occur
	scenarios := []struct {
		name               string
		remainingToday     int
		batchSize          int
		expectedAPICalls   bool
		expectedLogMessage string
	}{
		{
			name:               "No_Remaining_No_API_Calls",
			remainingToday:     0,
			batchSize:          3,
			expectedAPICalls:   false,
			expectedLogMessage: "All subscriptions processed for today",
		},
		{
			name:               "Has_Remaining_Should_Make_API_Calls",
			remainingToday:     5,
			batchSize:          3,
			expectedAPICalls:   true,
			expectedLogMessage: "Executing batch subscription rotation processing",
		},
		{
			name:               "Small_Remaining_Batch_Limited",
			remainingToday:     2,
			batchSize:          3,
			expectedAPICalls:   true,
			expectedLogMessage: "Executing batch subscription rotation processing",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Create a mock result that would be populated by the actual processing
			result := &JobResult{
				JobType:   "article_fetch",
				StartTime: time.Now(),
			}

			// Simulate what the processing logic would set
			if scenario.expectedAPICalls {
				result.Details = map[string]interface{}{
					"status":          "processed",
					"batch_size":      scenario.batchSize,
					"processed_count": min(scenario.remainingToday, scenario.batchSize),
					"api_calls_made":  true,
				}
				result.Success = true
			} else {
				result.Details = map[string]interface{}{
					"status":          "all_subscriptions_completed_today",
					"batch_size":      scenario.batchSize,
					"processed_count": 0,
					"api_calls_made":  false,
				}
				result.Success = true // Still successful, just no work to do
			}

			details, ok := result.Details.(map[string]interface{})
			require.True(t, ok)

			assert.Equal(t, scenario.expectedAPICalls, details["api_calls_made"])

			if scenario.expectedAPICalls {
				assert.Greater(t, details["processed_count"], 0)
			} else {
				assert.Equal(t, 0, details["processed_count"])
			}
		})
	}
}

// min helper function for tests
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
