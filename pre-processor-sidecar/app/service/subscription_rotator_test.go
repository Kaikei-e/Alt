package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSubscriptionRotator(t *testing.T) {
	// Clear environment variable to test defaults
	originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
	defer func() {
		if originalEnv != "" {
			os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
		} else {
			os.Unsetenv("ROTATION_INTERVAL_MINUTES")
		}
	}()
	os.Unsetenv("ROTATION_INTERVAL_MINUTES")

	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)

	assert.NotNil(t, rotator)
	assert.Equal(t, 30, rotator.intervalMinutes) // Changed from 20 to 30 minutes default
	assert.Equal(t, 2, rotator.maxDaily)         // Should be 2 by default (changed from 1)
	assert.Equal(t, 0, rotator.currentIndex)
	assert.Empty(t, rotator.subscriptions)
	assert.Empty(t, rotator.lastProcessed)
}

func TestLoadSubscriptions(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Test empty subscriptions
	err := rotator.LoadSubscriptions(ctx, []uuid.UUID{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no subscriptions provided")

	// Test loading valid subscriptions
	subs := []uuid.UUID{
		uuid.New(),
		uuid.New(),
		uuid.New(),
	}

	err = rotator.LoadSubscriptions(ctx, subs)
	assert.NoError(t, err)
	assert.Equal(t, len(subs), len(rotator.subscriptions))

	// Verify all UUIDs are present (order may be different due to shuffle)
	for _, originalSub := range subs {
		found := false
		for _, loadedSub := range rotator.subscriptions {
			if originalSub == loadedSub {
				found = true
				break
			}
		}
		assert.True(t, found, "Original subscription %s not found in loaded subscriptions", originalSub)
	}
}

func TestGetNextSubscription(t *testing.T) {
	// Set MAX_DAILY_ROTATIONS=1 to maintain original test behavior
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "1")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Test with no subscriptions
	sub, hasNext := rotator.GetNextSubscription()
	assert.Equal(t, uuid.Nil, sub)
	assert.False(t, hasNext)

	// Load test subscriptions
	subs := []uuid.UUID{
		uuid.New(),
		uuid.New(),
	}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Test getting subscriptions in order
	sub1, hasNext1 := rotator.GetNextSubscription()
	assert.NotEqual(t, uuid.Nil, sub1)
	assert.True(t, hasNext1)
	assert.Equal(t, 1, rotator.currentIndex)

	sub2, hasNext2 := rotator.GetNextSubscription()
	assert.NotEqual(t, uuid.Nil, sub2)
	assert.True(t, hasNext2)
	assert.Equal(t, 2, rotator.currentIndex)
	assert.NotEqual(t, sub1, sub2)

	// Test when all subscriptions processed
	sub3, hasNext3 := rotator.GetNextSubscription()
	assert.Equal(t, uuid.Nil, sub3)
	assert.False(t, hasNext3)
}

func TestDailyReset(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load subscriptions
	subs := []uuid.UUID{uuid.New(), uuid.New()}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Process one subscription
	_, hasNext := rotator.GetNextSubscription()
	assert.True(t, hasNext)
	assert.Equal(t, 1, rotator.currentIndex)

	// Simulate next day by changing lastResetDate
	rotator.mu.Lock()
	rotator.lastResetDate = time.Now().Add(-25 * time.Hour).Truncate(24 * time.Hour)
	rotator.mu.Unlock()

	// Next call should trigger daily reset
	sub, hasNext := rotator.GetNextSubscription()
	assert.NotEqual(t, uuid.Nil, sub)
	assert.True(t, hasNext)
	assert.Equal(t, 1, rotator.currentIndex) // Should be reset and then incremented
}

func TestGetStats(t *testing.T) {
	// Set MAX_DAILY_ROTATIONS=1 to maintain original test expectations
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "1")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load subscriptions
	subs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Initial stats
	stats := rotator.GetStats()
	assert.Equal(t, 3, stats.TotalSubscriptions)
	assert.Equal(t, 0, stats.ProcessedToday)
	assert.Equal(t, 3, stats.RemainingToday)

	// Process one subscription
	_, _ = rotator.GetNextSubscription()

	// Updated stats
	stats = rotator.GetStats()
	assert.Equal(t, 3, stats.TotalSubscriptions)
	assert.Equal(t, 1, stats.ProcessedToday)
	assert.Equal(t, 2, stats.RemainingToday)
}

func TestIsReadyForNext(t *testing.T) {
	// Clear environment to ensure default 30-minute interval
	originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
	defer func() {
		if originalEnv != "" {
			os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
		} else {
			os.Unsetenv("ROTATION_INTERVAL_MINUTES")
		}
	}()
	os.Unsetenv("ROTATION_INTERVAL_MINUTES")

	rotator := NewSubscriptionRotator(slog.Default())

	// Should be ready when no previous processing
	assert.True(t, rotator.IsReadyForNext())

	// Add a recent processing time
	testUUID := uuid.New()
	rotator.mu.Lock()
	rotator.lastProcessed[testUUID] = time.Now().Add(-15 * time.Minute) // 15 minutes ago
	rotator.mu.Unlock()

	// Should not be ready (interval is 30 minutes)
	assert.False(t, rotator.IsReadyForNext())

	// Update the same UUID with older processing time (simulating time passage)
	rotator.mu.Lock()
	rotator.lastProcessed[testUUID] = time.Now().Add(-35 * time.Minute) // 35 minutes ago
	rotator.mu.Unlock()

	// Should be ready now
	assert.True(t, rotator.IsReadyForNext())
}

func TestSetInterval(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())

	// Test valid interval
	rotator.SetInterval(30)
	assert.Equal(t, 30, rotator.GetInterval())

	// Test minimum bound
	rotator.SetInterval(0)
	assert.Equal(t, 1, rotator.GetInterval())

	// Test maximum bound
	rotator.SetInterval(300)
	assert.Equal(t, 240, rotator.GetInterval())
}

func TestGetProcessingStatus(t *testing.T) {
	// Set MAX_DAILY_ROTATIONS=1 to maintain original test expectations
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "1")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load subscriptions
	subs := []uuid.UUID{uuid.New(), uuid.New()}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Initial status
	status := rotator.GetProcessingStatus()
	assert.Contains(t, status, "Processing 0/2")

	// Process one subscription
	_, _ = rotator.GetNextSubscription()
	status = rotator.GetProcessingStatus()
	assert.Contains(t, status, "Processing 1/2")

	// Complete all subscriptions
	_, _ = rotator.GetNextSubscription()
	status = rotator.GetProcessingStatus()
	assert.Contains(t, status, "Completed 2/2")
}

func TestRotatorConcurrency(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load subscriptions
	subs := make([]uuid.UUID, 10)
	for i := range subs {
		subs[i] = uuid.New()
	}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Test concurrent access
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 5; i++ {
			rotator.GetNextSubscription()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 5; i++ {
			rotator.GetStats()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify state is consistent
	stats := rotator.GetStats()
	assert.True(t, stats.ProcessedToday >= 0)
	assert.True(t, stats.ProcessedToday <= 10)
}

// Test for daily rotation completion logic with MAX_DAILY_ROTATIONS=2
func TestHasCompletedDailyRotationWithTwoRotations(t *testing.T) {
	// Set environment variable for MAX_DAILY_ROTATIONS=2
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "2")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load 40 subscriptions (realistic number)
	subs := make([]uuid.UUID, 40)
	for i := range subs {
		subs[i] = uuid.New()
	}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Test initial state: should not be completed
	completed := rotator.hasCompletedDailyRotation()
	assert.False(t, completed, "Should not be completed initially")

	// Process 40 subscriptions (1st rotation)
	for i := 0; i < 40; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub, "Should have next subscription at index %d", i)
		assert.True(t, hasNext, "Should have next subscription at index %d", i)
	}

	// Should not be completed yet (need 2nd rotation)
	completed = rotator.hasCompletedDailyRotation()
	assert.False(t, completed, "Should not be completed after 1st rotation")

	// Process another 40 subscriptions (2nd rotation)
	for i := 0; i < 40; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub, "Should have next subscription in 2nd rotation at index %d", i)
		assert.True(t, hasNext, "Should have next subscription in 2nd rotation at index %d", i)
	}

	// Now should be completed (80 total: 40 subs × 2 rotations)
	completed = rotator.hasCompletedDailyRotation()
	assert.True(t, completed, "Should be completed after 2 full rotations")

	// Trying to get next subscription should return nil
	sub, hasNext := rotator.GetNextSubscription()
	assert.Equal(t, uuid.Nil, sub, "Should return nil when completed")
	assert.False(t, hasNext, "Should return false when completed")
}

// Test batch processing capacity validation
func TestBatchProcessingCapacityValidation(t *testing.T) {
	tests := []struct {
		name               string
		subscriptions      int
		maxDailyRotations  int
		batchSize          int
		intervalMinutes    int
		expectedValidation bool
		description        string
	}{
		{
			name:               "Current problematic config",
			subscriptions:      40,
			maxDailyRotations:  2,
			batchSize:          2,
			intervalMinutes:    30,
			expectedValidation: true, // 48 intervals × 2 batch = 96 capacity ≥ 80 required
			description:        "40 subs × 2 rotations = 80, 48 intervals × 2 batch = 96 capacity",
		},
		{
			name:               "Insufficient capacity",
			subscriptions:      100,
			maxDailyRotations:  2,
			batchSize:          1,
			intervalMinutes:    60,
			expectedValidation: false, // 24 intervals × 1 batch = 24 capacity < 200 required
			description:        "100 subs × 2 rotations = 200, 24 intervals × 1 batch = 24 capacity",
		},
		{
			name:               "Optimal config",
			subscriptions:      40,
			maxDailyRotations:  2,
			batchSize:          3,
			intervalMinutes:    30,
			expectedValidation: true, // 48 intervals × 3 batch = 144 capacity ≥ 80 required
			description:        "40 subs × 2 rotations = 80, 48 intervals × 3 batch = 144 capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredProcessing := tt.subscriptions * tt.maxDailyRotations
			dailyIntervals := (24 * 60) / tt.intervalMinutes
			dailyCapacity := dailyIntervals * tt.batchSize

			isValid := dailyCapacity >= requiredProcessing

			assert.Equal(t, tt.expectedValidation, isValid,
				"Capacity validation failed: %s. Required: %d, Capacity: %d",
				tt.description, requiredProcessing, dailyCapacity)

			t.Logf("Test: %s", tt.name)
			t.Logf("Required processing per day: %d", requiredProcessing)
			t.Logf("Daily intervals (%d min): %d", tt.intervalMinutes, dailyIntervals)
			t.Logf("Daily capacity (batch size %d): %d", tt.batchSize, dailyCapacity)
			t.Logf("Valid configuration: %v", isValid)
		})
	}
}

// Test 24-hour simulation with realistic timing
func TestTwentyFourHourSimulation(t *testing.T) {
	// Set environment variable for MAX_DAILY_ROTATIONS=2
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "2")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load 5 subscriptions for faster test
	subs := make([]uuid.UUID, 5)
	for i := range subs {
		subs[i] = uuid.New()
	}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Simulate 30-minute intervals for 24 hours (48 intervals)
	totalIntervals := 48
	batchSize := 2

	processedCount := 0
	expectedTotal := len(subs) * 2 // 5 subs × 2 rotations = 10

	for interval := 0; interval < totalIntervals; interval++ {
		// Check if we can still process
		batch := rotator.GetNextSubscriptionBatch(batchSize)
		if len(batch) == 0 {
			// All processing completed for the day
			t.Logf("All processing completed at interval %d/%d", interval+1, totalIntervals)
			break
		}

		// Process the batch
		for _, subID := range batch {
			// Verify it's a valid subscription
			found := false
			for _, originalSub := range subs {
				if originalSub == subID {
					found = true
					break
				}
			}
			assert.True(t, found, "Invalid subscription ID in batch: %s", subID)
			processedCount++
		}

		t.Logf("Interval %d: processed batch of %d, total processed: %d/%d",
			interval+1, len(batch), processedCount, expectedTotal)
	}

	// Verify final results
	stats := rotator.GetStats()
	assert.Equal(t, expectedTotal, processedCount,
		"Should have processed all subscriptions twice")
	assert.Equal(t, len(subs), stats.TotalSubscriptions)
	assert.Equal(t, 0, stats.RemainingToday,
		"Should have no remaining subscriptions")
}

// Test configuration validation helper function
func TestValidateRotationConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		subscriptions   int
		maxDaily        int
		batchSize       int
		intervalMinutes int
		expectValid     bool
		expectedError   string
	}{
		{
			name:            "Valid current config",
			subscriptions:   40,
			maxDaily:        2,
			batchSize:       2,
			intervalMinutes: 30,
			expectValid:     true,
		},
		{
			name:            "Invalid - insufficient capacity",
			subscriptions:   100,
			maxDaily:        3,
			batchSize:       1,
			intervalMinutes: 60,
			expectValid:     false,
			expectedError:   "insufficient daily capacity",
		},
		{
			name:            "Valid optimized config",
			subscriptions:   40,
			maxDaily:        2,
			batchSize:       3,
			intervalMinutes: 30,
			expectValid:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRotationConfiguration(tt.subscriptions, tt.maxDaily, tt.batchSize, tt.intervalMinutes)

			if tt.expectValid {
				assert.NoError(t, err, "Configuration should be valid")
			} else {
				assert.Error(t, err, "Configuration should be invalid")
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			}
		})
	}
}

// Test configurable interval via environment variable
func TestConfigurableIntervalFromEnvironment(t *testing.T) {
	// Test default interval (30 minutes)
	t.Run("default_interval", func(t *testing.T) {
		// Clear environment variable
		originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
		defer func() {
			if originalEnv != "" {
				os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
			} else {
				os.Unsetenv("ROTATION_INTERVAL_MINUTES")
			}
		}()
		os.Unsetenv("ROTATION_INTERVAL_MINUTES")

		rotator := NewSubscriptionRotator(slog.Default())
		assert.Equal(t, 30, rotator.intervalMinutes, "Default interval should be 30 minutes")
	})

	// Test custom interval from environment
	t.Run("custom_interval_from_env", func(t *testing.T) {
		originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
		defer func() {
			if originalEnv != "" {
				os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
			} else {
				os.Unsetenv("ROTATION_INTERVAL_MINUTES")
			}
		}()

		// Test 25 minute interval
		os.Setenv("ROTATION_INTERVAL_MINUTES", "25")
		rotator := NewSubscriptionRotator(slog.Default())
		assert.Equal(t, 25, rotator.intervalMinutes, "Should use environment variable interval")
	})

	// Test invalid environment values
	t.Run("invalid_env_values", func(t *testing.T) {
		originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
		defer func() {
			if originalEnv != "" {
				os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
			} else {
				os.Unsetenv("ROTATION_INTERVAL_MINUTES")
			}
		}()

		cases := []struct {
			name     string
			envValue string
			expected int
		}{
			{"non_numeric", "invalid", 30},
			{"zero", "0", 30},
			{"negative", "-5", 30},
			{"too_large", "500", 30},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				os.Setenv("ROTATION_INTERVAL_MINUTES", tc.envValue)
				rotator := NewSubscriptionRotator(slog.Default())
				assert.Equal(t, tc.expected, rotator.intervalMinutes,
					"Invalid environment value should fallback to default")
			})
		}
	})
}

// Test timing calculations with configurable interval
func TestTimingCalculationsWithConfigurableInterval(t *testing.T) {
	originalEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
	defer func() {
		if originalEnv != "" {
			os.Setenv("ROTATION_INTERVAL_MINUTES", originalEnv)
		} else {
			os.Unsetenv("ROTATION_INTERVAL_MINUTES")
		}
	}()

	// Test with 30-minute interval
	os.Setenv("ROTATION_INTERVAL_MINUTES", "30")
	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load test subscriptions
	subs := []uuid.UUID{uuid.New(), uuid.New()}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Process one subscription and check timing
	beforeTime := time.Now()
	_, hasNext := rotator.GetNextSubscription()
	assert.True(t, hasNext)

	// Check if IsReadyForNext respects the new interval
	assert.False(t, rotator.IsReadyForNext(), "Should not be ready immediately")

	// Simulate time passing (less than 30 minutes)
	rotator.mu.Lock()
	for uuid, _ := range rotator.lastProcessed {
		rotator.lastProcessed[uuid] = beforeTime.Add(-25 * time.Minute)
		break
	}
	rotator.mu.Unlock()
	assert.False(t, rotator.IsReadyForNext(), "Should not be ready before 30 minutes")

	// Simulate 30+ minutes passing
	rotator.mu.Lock()
	for uuid, _ := range rotator.lastProcessed {
		rotator.lastProcessed[uuid] = beforeTime.Add(-35 * time.Minute)
		break
	}
	rotator.mu.Unlock()
	assert.True(t, rotator.IsReadyForNext(), "Should be ready after 30+ minutes")
}

// Test compatibility with schedule handler expectations
func TestScheduleHandlerCompatibility(t *testing.T) {
	// This test verifies that the rotator interval matches what schedule handler expects
	originalRotationEnv := os.Getenv("ROTATION_INTERVAL_MINUTES")
	originalDailyEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalRotationEnv != "" {
			os.Setenv("ROTATION_INTERVAL_MINUTES", originalRotationEnv)
		} else {
			os.Unsetenv("ROTATION_INTERVAL_MINUTES")
		}
		if originalDailyEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalDailyEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()

	// Set to 30 minutes (matching schedule handler default)
	os.Setenv("ROTATION_INTERVAL_MINUTES", "30")
	// Set to 2 rotations per day (matching production config)
	os.Setenv("MAX_DAILY_ROTATIONS", "2")
	rotator := NewSubscriptionRotator(slog.Default())

	// Verify interval matches expected value
	assert.Equal(t, 30, rotator.GetInterval(), "Rotator interval should match schedule handler")

	// Verify calculations work correctly for realistic setup
	ctx := context.Background()
	subs := make([]uuid.UUID, 46) // Realistic subscription count
	for i := range subs {
		subs[i] = uuid.New()
	}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	stats := rotator.GetStats()
	// With 46 subscriptions, 30-minute intervals, and batch size 2:
	// Daily intervals: 24*60/30 = 48
	// Daily capacity with batch size 2: 48*2 = 96
	// Required for 2 rotations: 46*2 = 92
	// Should be feasible: 96 >= 92
	assert.Equal(t, 46, stats.TotalSubscriptions)
	assert.Equal(t, 92, stats.RemainingToday) // 46 * 2 rotations
}

// Helper function to validate rotation configuration
func validateRotationConfiguration(subscriptions, maxDaily, batchSize, intervalMinutes int) error {
	requiredProcessing := subscriptions * maxDaily
	dailyIntervals := (24 * 60) / intervalMinutes
	dailyCapacity := dailyIntervals * batchSize

	if dailyCapacity < requiredProcessing {
		return fmt.Errorf("insufficient daily capacity: need %d, have %d (intervals: %d, batch: %d)",
			requiredProcessing, dailyCapacity, dailyIntervals, batchSize)
	}

	return nil
}

// TestDailyResetCurrentIndex tests that currentIndex is properly reset during daily rotation
// This test verifies the fix for the bug where second daily rotation cycle doesn't start
func TestDailyResetCurrentIndex(t *testing.T) {
	// Set MAX_DAILY_ROTATIONS=2 for this test
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "2")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Load 3 test subscriptions
	subs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Process all subscriptions in first cycle (3 subscriptions)
	for i := 0; i < 3; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub)
		assert.True(t, hasNext)
	}
	assert.Equal(t, 3, rotator.currentIndex) // After first cycle

	// Continue processing second cycle (should process 3 more)
	for i := 0; i < 3; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub)
		assert.True(t, hasNext) // All should be true since we have 2 cycles (MAX_DAILY_ROTATIONS=2)
	}
	assert.Equal(t, 6, rotator.currentIndex) // After both cycles (3*2=6)

	// Verify no more subscriptions are available for today
	subAfterCycles, hasNextAfterCycles := rotator.GetNextSubscription()
	assert.Equal(t, uuid.Nil, subAfterCycles)
	assert.False(t, hasNextAfterCycles) // Should be false since all subscriptions processed for today

	// Simulate next day - this should reset currentIndex to 0
	rotator.mu.Lock()
	originalResetDate := rotator.lastResetDate
	// Set lastResetDate to yesterday (in timezone)
	yesterday := time.Now().In(rotator.timezone).Add(-24 * time.Hour)
	year, month, day := yesterday.Date()
	rotator.lastResetDate = time.Date(year, month, day, 0, 0, 0, 0, rotator.timezone)
	rotator.mu.Unlock()

	// Get next subscription should trigger daily reset
	subAfterReset, hasNextAfterReset := rotator.GetNextSubscription()
	assert.NotEqual(t, uuid.Nil, subAfterReset)
	assert.True(t, hasNextAfterReset)

	// CRITICAL TEST: currentIndex should be reset to 1 after daily reset
	// This is the bug we're fixing - currently it stays at 6
	rotator.mu.RLock()
	currentIndexAfterReset := rotator.currentIndex
	rotator.mu.RUnlock()

	assert.Equal(t, 1, currentIndexAfterReset,
		"currentIndex should be reset to 1 after daily reset, but was %d", currentIndexAfterReset)

	// Verify we can process full cycles again
	stats := rotator.GetStats()
	assert.Equal(t, 1, stats.ProcessedToday, "Should show 1 processed after reset")
	assert.Equal(t, 5, stats.RemainingToday, "Should show 5 remaining (3*2-1)")

	t.Logf("Original reset date: %s", originalResetDate.Format("2006-01-02 15:04:05"))
	t.Logf("Current index after reset: %d", currentIndexAfterReset)
	t.Logf("Stats after reset: processed=%d, remaining=%d", stats.ProcessedToday, stats.RemainingToday)
}

// TestProduction46SubscriptionsTwiceDaily simulates the production scenario with 46 subscriptions
// This integration test verifies that all 46 subscriptions can be processed twice per day (92 total)
func TestProduction46SubscriptionsTwiceDaily(t *testing.T) {
	// Set MAX_DAILY_ROTATIONS=2 for production scenario
	originalEnv := os.Getenv("MAX_DAILY_ROTATIONS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MAX_DAILY_ROTATIONS", originalEnv)
		} else {
			os.Unsetenv("MAX_DAILY_ROTATIONS")
		}
	}()
	os.Setenv("MAX_DAILY_ROTATIONS", "2")

	rotator := NewSubscriptionRotator(slog.Default())
	ctx := context.Background()

	// Create 46 subscriptions (production scenario)
	subs := make([]uuid.UUID, 46)
	for i := range subs {
		subs[i] = uuid.New()
	}

	err := rotator.LoadSubscriptions(ctx, subs)
	require.NoError(t, err)

	// Initial stats check
	initialStats := rotator.GetStats()
	assert.Equal(t, 46, initialStats.TotalSubscriptions)
	assert.Equal(t, 0, initialStats.ProcessedToday)
	assert.Equal(t, 92, initialStats.RemainingToday) // 46 * 2 cycles

	t.Logf("Initial stats: total=%d, remaining=%d", initialStats.TotalSubscriptions, initialStats.RemainingToday)

	// Process all subscriptions in the first cycle (46 subscriptions)
	processedCount := 0
	for i := 0; i < 46; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub, "Expected subscription at position %d", i)
		assert.True(t, hasNext, "Should have next subscription at position %d", i)
		processedCount++
	}

	// Check stats after first cycle
	statsAfterFirstCycle := rotator.GetStats()
	assert.Equal(t, 46, statsAfterFirstCycle.ProcessedToday)
	assert.Equal(t, 46, statsAfterFirstCycle.RemainingToday) // Still 46 remaining for second cycle

	t.Logf("After first cycle: processed=%d, remaining=%d", statsAfterFirstCycle.ProcessedToday, statsAfterFirstCycle.RemainingToday)

	// Process all subscriptions in the second cycle (another 46 subscriptions)
	for i := 0; i < 46; i++ {
		sub, hasNext := rotator.GetNextSubscription()
		assert.NotEqual(t, uuid.Nil, sub, "Expected subscription at second cycle position %d", i)
		// All should have hasNext=true for the processing within cycles
		assert.True(t, hasNext, "Should have next subscription at second cycle position %d", i)
		processedCount++
	}

	// Final stats check
	finalStats := rotator.GetStats()
	assert.Equal(t, 92, finalStats.ProcessedToday) // 46 * 2 cycles
	assert.Equal(t, 0, finalStats.RemainingToday)  // No more remaining
	assert.Equal(t, 92, processedCount)            // Total processed count

	t.Logf("Final stats: processed=%d, remaining=%d, total_processed=%d",
		finalStats.ProcessedToday, finalStats.RemainingToday, processedCount)

	// Verify no more subscriptions are available for today
	noMoreSub, noMoreHasNext := rotator.GetNextSubscription()
	assert.Equal(t, uuid.Nil, noMoreSub)
	assert.False(t, noMoreHasNext)

	// Test daily reset scenario
	rotator.mu.Lock()
	yesterday := time.Now().In(rotator.timezone).Add(-24 * time.Hour)
	year, month, day := yesterday.Date()
	rotator.lastResetDate = time.Date(year, month, day, 0, 0, 0, 0, rotator.timezone)
	rotator.mu.Unlock()

	// After reset, should be able to process again
	subAfterReset, hasNextAfterReset := rotator.GetNextSubscription()
	assert.NotEqual(t, uuid.Nil, subAfterReset)
	assert.True(t, hasNextAfterReset)

	// Stats should be reset
	statsAfterReset := rotator.GetStats()
	assert.Equal(t, 1, statsAfterReset.ProcessedToday)  // Just processed 1
	assert.Equal(t, 91, statsAfterReset.RemainingToday) // 92 - 1 = 91 remaining

	t.Logf("After daily reset: processed=%d, remaining=%d",
		statsAfterReset.ProcessedToday, statsAfterReset.RemainingToday)
}
