package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSubscriptionRotator(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)

	assert.NotNil(t, rotator)
	assert.Equal(t, 20, rotator.intervalMinutes)
	assert.Equal(t, 40, rotator.maxDaily)
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
	rotator := NewSubscriptionRotator(slog.Default())
	
	// Should be ready when no previous processing
	assert.True(t, rotator.IsReadyForNext())

	// Add a recent processing time
	rotator.mu.Lock()
	rotator.lastProcessed[uuid.New()] = time.Now().Add(-10 * time.Minute) // 10 minutes ago
	rotator.mu.Unlock()

	// Should not be ready (interval is 20 minutes)
	assert.False(t, rotator.IsReadyForNext())

	// Add an older processing time
	rotator.mu.Lock()
	rotator.lastProcessed[uuid.New()] = time.Now().Add(-25 * time.Minute) // 25 minutes ago
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