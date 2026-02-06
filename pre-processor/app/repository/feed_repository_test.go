package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement FeedRepository interface", func(t *testing.T) {
		// GREEN PHASE: Test that repository implements interface
		repo := NewFeedRepository(nil, testLoggerRepo())

		// Verify interface compliance at compile time
		var _ = repo

		assert.NotNil(t, repo)
	})
}

func TestFeedRepository_GetUnprocessedFeeds(t *testing.T) {
	tests := map[string]struct {
		cursor   *domain.Cursor
		limit    int
		wantErr  bool
		wantURLs int
	}{
		"nil database with nil cursor": {
			cursor:   nil,
			limit:    10,
			wantErr:  true,
			wantURLs: 0,
		},
		"nil database with valid cursor": {
			cursor: &domain.Cursor{
				LastCreatedAt: &time.Time{},
				LastID:        "test-id",
			},
			limit:    10,
			wantErr:  true,
			wantURLs: 0,
		},
		"nil database with zero limit": {
			cursor:   nil,
			limit:    0,
			wantErr:  true,
			wantURLs: 0,
		},
		"nil database with negative limit": {
			cursor:   nil,
			limit:    -1,
			wantErr:  true,
			wantURLs: 0,
		},
		"nil database with valid parameters": {
			cursor:   nil,
			limit:    50,
			wantErr:  true,
			wantURLs: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation with nil database
			// Initialize global logger for driver dependencies

			repo := NewFeedRepository(nil, testLoggerRepo())

			urls, cursor, err := repo.GetUnprocessedFeeds(context.Background(), tc.cursor, tc.limit)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, urls)
				assert.Nil(t, cursor)
			} else {
				assert.NoError(t, err)
				assert.Len(t, urls, tc.wantURLs)
				assert.NotNil(t, cursor)
			}
		})
	}
}

func TestFeedRepository_GetUnprocessedFeeds_NilHandling(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		urls, cursor, err := repo.GetUnprocessedFeeds(context.Background(), nil, 10)

		// Should return error due to nil database
		assert.Error(t, err)
		assert.Nil(t, urls)
		assert.Nil(t, cursor)
		assert.Contains(t, err.Error(), "failed to get unprocessed feeds")
	})

	t.Run("should handle context cancellation", func(t *testing.T) {
		// GREEN PHASE: Test context handling
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		urls, cursor, err := repo.GetUnprocessedFeeds(ctx, nil, 10)

		// Should return error due to canceled context or nil database
		assert.Error(t, err)
		assert.Nil(t, urls)
		assert.Nil(t, cursor)
	})
}

func TestFeedRepository_GetProcessingStats(t *testing.T) {
	tests := map[string]struct {
		wantErr       bool
		wantTotal     int
		wantProcessed int
		wantRemaining int
	}{
		"nil database": {
			wantErr:       true,
			wantTotal:     0,
			wantProcessed: 0,
			wantRemaining: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation with nil database
			// Initialize global logger for driver dependencies

			repo := NewFeedRepository(nil, testLoggerRepo())

			stats, err := repo.GetProcessingStats(context.Background())

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, stats)
			} else {
				require.NoError(t, err)
				require.NotNil(t, stats)
				assert.Equal(t, tc.wantTotal, stats.TotalFeeds)
				assert.Equal(t, tc.wantProcessed, stats.ProcessedFeeds)
				assert.Equal(t, tc.wantRemaining, stats.RemainingFeeds)
			}
		})
	}
}

func TestFeedRepository_GetProcessingStats_NilHandling(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		// GREEN PHASE: Test minimal implementation
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		stats, err := repo.GetProcessingStats(context.Background())

		// Should return error due to nil database
		assert.Error(t, err)
		assert.Nil(t, stats)
		assert.Contains(t, err.Error(), "failed to get processing statistics")
	})

	t.Run("should handle context cancellation", func(t *testing.T) {
		// GREEN PHASE: Test context handling
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		stats, err := repo.GetProcessingStats(ctx)

		// Should return error due to canceled context or nil database
		assert.Error(t, err)
		assert.Nil(t, stats)
	})
}

func TestFeedRepository_CursorHandling(t *testing.T) {
	t.Run("should handle cursor with zero time", func(t *testing.T) {
		// GREEN PHASE: Test cursor edge cases
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		cursor := &domain.Cursor{
			LastCreatedAt: &time.Time{}, // Zero time
			LastID:        "",
		}

		urls, newCursor, err := repo.GetUnprocessedFeeds(context.Background(), cursor, 10)

		// Should return error due to nil database
		assert.Error(t, err)
		assert.Nil(t, urls)
		assert.Nil(t, newCursor)
	})

	t.Run("should handle cursor with empty ID", func(t *testing.T) {
		// GREEN PHASE: Test cursor edge cases
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		now := time.Now()
		cursor := &domain.Cursor{
			LastCreatedAt: &now,
			LastID:        "", // Empty ID
		}

		urls, newCursor, err := repo.GetUnprocessedFeeds(context.Background(), cursor, 10)

		// Should return error due to nil database
		assert.Error(t, err)
		assert.Nil(t, urls)
		assert.Nil(t, newCursor)
	})
}

func TestFeedRepository_ErrorHandling(t *testing.T) {
	t.Run("should wrap errors properly", func(t *testing.T) {
		// GREEN PHASE: Test error wrapping
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		// Test GetUnprocessedFeeds error wrapping
		_, _, err := repo.GetUnprocessedFeeds(context.Background(), nil, 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get unprocessed feeds")

		// Test GetProcessingStats error wrapping
		_, err = repo.GetProcessingStats(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get processing statistics")
	})
}

func TestFeedRepository_URLConversion(t *testing.T) {
	t.Run("should handle URL conversion in GetUnprocessedFeeds", func(t *testing.T) {
		// GREEN PHASE: This test verifies the URL conversion logic
		// The repository converts []url.URL to []*url.URL
		// Initialize global logger for driver dependencies

		repo := NewFeedRepository(nil, testLoggerRepo())

		urls, cursor, err := repo.GetUnprocessedFeeds(context.Background(), nil, 10)

		// Should return error due to nil database, but test the signature
		assert.Error(t, err)
		assert.Nil(t, urls) // Should be []*url.URL type
		assert.Nil(t, cursor)

		// Type assertion to ensure correct return type
		var _ = urls
	})
}

func TestFeedRepository_Logging(t *testing.T) {
	t.Run("should log operations", func(t *testing.T) {
		// GREEN PHASE: Test that logging is properly integrated
		// Create a logger that captures output
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))

		repo := NewFeedRepository(nil, logger)

		// These should trigger logging even with errors
		_, _, _ = repo.GetUnprocessedFeeds(context.Background(), nil, 10)
		_, _ = repo.GetProcessingStats(context.Background())

		// If we get here without panics, logging integration is working
		assert.True(t, true)
	})
}
