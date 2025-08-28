// ABOUTME: This file tests the schedule handler functionality
// ABOUTME: Following TDD principles with focus on testable components

package handler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	handler := NewScheduleHandler(nil, nil, nil)

	// Test that triggering succeeds at method level
	err := handler.TriggerSubscriptionSync()
	assert.NoError(t, err)

	// Give a moment for goroutine to start and potentially panic
	time.Sleep(1 * time.Millisecond)
	
	// If we reach here, the trigger method worked (goroutine execution is separate)
}

func TestScheduleHandler_TriggerArticleFetch_NotRunning(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	// Test that triggering succeeds at method level
	err := handler.TriggerArticleFetch()
	assert.NoError(t, err)

	// Give a moment for goroutine to start and potentially panic
	time.Sleep(1 * time.Millisecond)
	
	// If we reach here, the trigger method worked (goroutine execution is separate)
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
		SubscriptionSyncEnabled:  true,
		ArticleFetchEnabled:      true,
		LastSubscriptionSync:     time.Now(),
		NextSubscriptionSync:     time.Now().Add(1 * time.Hour),
		LastArticleFetch:         time.Now(),
		NextArticleFetch:         time.Now().Add(30 * time.Minute),
		SubscriptionSyncRunning:  false,
		ArticleFetchRunning:      false,
		TotalSubscriptionSyncs:   10,
		TotalArticleFetches:      50,
		FailedSubscriptionSyncs:  1,
		FailedArticleFetches:     2,
		LastError:               "",
	}

	assert.True(t, status.SubscriptionSyncEnabled)
	assert.True(t, status.ArticleFetchEnabled)
	assert.False(t, status.SubscriptionSyncRunning)
	assert.False(t, status.ArticleFetchRunning)
	assert.Equal(t, int64(10), status.TotalSubscriptionSyncs)
	assert.Equal(t, int64(50), status.TotalArticleFetches)
}