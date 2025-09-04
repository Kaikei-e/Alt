package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimezoneHandling tests timezone-aware date reset functionality
func TestTimezoneHandling(t *testing.T) {
	tests := []struct {
		name                   string
		timezone               string
		mockTime               time.Time
		lastResetDate         time.Time
		expectedShouldReset    bool
		description           string
	}{
		{
			name:                   "JST_SameDay",
			timezone:              "Asia/Tokyo", 
			mockTime:              time.Date(2025, 9, 3, 14, 30, 0, 0, time.UTC), // 23:30 JST
			lastResetDate:        time.Date(2025, 9, 3, 0, 0, 0, 0, mustLoadLocation("Asia/Tokyo")),
			expectedShouldReset:   false,
			description:          "Same day in JST should not reset",
		},
		{
			name:                   "JST_NextDay", 
			timezone:              "Asia/Tokyo",
			mockTime:              time.Date(2025, 9, 4, 1, 30, 0, 0, time.UTC), // 10:30 JST next day
			lastResetDate:        time.Date(2025, 9, 3, 0, 0, 0, 0, mustLoadLocation("Asia/Tokyo")),
			expectedShouldReset:   true,
			description:          "Next day in JST should reset",
		},
		{
			name:                   "UTC_SameDay",
			timezone:              "UTC",
			mockTime:              time.Date(2025, 9, 3, 14, 30, 0, 0, time.UTC),
			lastResetDate:        time.Date(2025, 9, 3, 0, 0, 0, 0, time.UTC),
			expectedShouldReset:   false,
			description:          "Same day in UTC should not reset",
		},
		{
			name:                   "UTC_NextDay",
			timezone:              "UTC", 
			mockTime:              time.Date(2025, 9, 4, 1, 30, 0, 0, time.UTC),
			lastResetDate:        time.Date(2025, 9, 3, 0, 0, 0, 0, time.UTC),
			expectedShouldReset:   true,
			description:          "Next day in UTC should reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set timezone environment variable
			os.Setenv("TZ", tt.timezone)
			
			// Create rotator
			rotator := NewSubscriptionRotator(slog.Default())
			
			// Set the last reset date manually
			rotator.lastResetDate = tt.lastResetDate
			
			// Test shouldResetDaily
			shouldReset := rotator.shouldResetDaily(tt.mockTime)
			
			assert.Equal(t, tt.expectedShouldReset, shouldReset, tt.description)
			
			// Clean up
			os.Unsetenv("TZ")
		})
	}
}

// TestDailyRotationReset tests the full daily rotation reset process
func TestDailyRotationReset(t *testing.T) {
	// Set JST timezone
	os.Setenv("TZ", "Asia/Tokyo")
	defer os.Unsetenv("TZ")
	
	rotator := NewSubscriptionRotator(slog.Default())
	
	// Set initial state
	rotator.currentIndex = 10
	jst := mustLoadLocation("Asia/Tokyo")
	yesterday := time.Date(2025, 9, 2, 0, 0, 0, 0, jst)
	rotator.lastResetDate = yesterday
	
	// Mock time for next day
	now := time.Date(2025, 9, 3, 10, 30, 0, 0, time.UTC) // 19:30 JST
	
	// Verify reset is needed
	assert.True(t, rotator.shouldResetDaily(now), "Should need reset for next day")
	
	// Perform reset
	rotator.resetDailyRotation(now)
	
	// Verify reset occurred
	assert.Equal(t, 0, rotator.currentIndex, "Index should be reset to 0")
	
	expectedResetDate := time.Date(2025, 9, 3, 0, 0, 0, 0, jst)
	assert.True(t, rotator.lastResetDate.Equal(expectedResetDate), 
		"Last reset date should be updated to today in JST")
	
	// Verify no more resets needed for same day
	assert.False(t, rotator.shouldResetDaily(now), "Should not need reset for same day")
}

// TestTimezoneInfo tests the timezone information reporting
func TestTimezoneInfo(t *testing.T) {
	os.Setenv("TZ", "Asia/Tokyo")
	defer os.Unsetenv("TZ")
	
	rotator := NewSubscriptionRotator(slog.Default())
	
	info := rotator.GetTimezoneInfo()
	
	// Check required fields exist
	require.Contains(t, info, "timezone_name")
	require.Contains(t, info, "current_time_utc")
	require.Contains(t, info, "current_time_local")
	require.Contains(t, info, "last_reset_date")
	require.Contains(t, info, "next_reset_time")
	require.Contains(t, info, "hours_until_reset")
	
	// Check timezone is correct
	assert.Contains(t, info["timezone_name"].(string), "Asia/Tokyo")
	
	// Check hours until reset is reasonable (0-24)
	hoursUntilReset := info["hours_until_reset"].(float64)
	assert.True(t, hoursUntilReset >= 0 && hoursUntilReset <= 24, 
		"Hours until reset should be between 0-24, got: %f", hoursUntilReset)
}

// TestEnvironmentTimezoneSettings tests different timezone configurations
func TestEnvironmentTimezoneSettings(t *testing.T) {
	tests := []struct {
		name               string
		envTZ              string
		expectedContains   string
	}{
		{
			name:             "Default_JST",
			envTZ:            "", // No TZ set, should default to Asia/Tokyo  
			expectedContains: "Asia/Tokyo",
		},
		{
			name:             "Explicit_UTC",
			envTZ:            "UTC",
			expectedContains: "UTC",
		},
		{
			name:             "Explicit_EST",
			envTZ:            "America/New_York", 
			expectedContains: "America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean environment first
			os.Unsetenv("TZ")
			
			// Set test environment
			if tt.envTZ != "" {
				os.Setenv("TZ", tt.envTZ)
				defer os.Unsetenv("TZ")
			}
			
			// Create rotator
			rotator := NewSubscriptionRotator(slog.Default())
			
			// Get timezone info
			info := rotator.GetTimezoneInfo()
			
			// Verify timezone
			timezoneName := info["timezone_name"].(string)
			assert.Contains(t, timezoneName, tt.expectedContains, 
				"Expected timezone %s, got %s", tt.expectedContains, timezoneName)
		})
	}
}

// Helper function to load timezone location
func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// TestSubscriptionCompletionLogic tests that subscriptions are properly marked as complete
func TestSubscriptionCompletionLogic(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())
	
	// Test with no subscriptions
	assert.True(t, rotator.hasCompletedDailyRotation(), "Should be complete with no subscriptions")
	
	// Load mock subscriptions
	subscriptions := []uuid.UUID{
		uuid.New(),
		uuid.New(),
		uuid.New(),
	}
	
	err := rotator.LoadSubscriptions(context.Background(), subscriptions)
	require.NoError(t, err)
	
	// Initially should not be complete
	assert.False(t, rotator.hasCompletedDailyRotation(), "Should not be complete initially")
	
	// Process all subscriptions
	for i := 0; i < len(subscriptions); i++ {
		_, hasNext := rotator.GetNextSubscription()
		assert.True(t, hasNext, "Should have next subscription for index %d", i)
	}
	
	// Now should be complete
	assert.True(t, rotator.hasCompletedDailyRotation(), "Should be complete after processing all")
	
	// Getting next subscription should return false
	_, hasNext := rotator.GetNextSubscription()
	assert.False(t, hasNext, "Should not have next subscription when complete")
}