package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"

	"pre-processor/models"
)

// Mock ArticleFetcher for testing
type MockArticleFetcher struct {
	fetchCount int
	fetchDelay time.Duration
	shouldFail bool
}

func (m *MockArticleFetcher) FetchArticle(ctx context.Context, urlStr string) (*models.Article, error) {
	m.fetchCount++
	
	if m.fetchDelay > 0 {
		select {
		case <-time.After(m.fetchDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	if m.shouldFail {
		return nil, errors.New("fetch failed")
	}
	
	return &models.Article{
		Title:   "Test Article",
		Content: "Test content",
		URL:     urlStr,
	}, nil
}

func (m *MockArticleFetcher) ValidateURL(urlStr string) error {
	return nil
}

func TestFeedWorkerPool_NewFeedWorkerPool(t *testing.T) {
	tests := []struct {
		name        string
		workers     int
		queueSize   int
		want        struct {
			workers   int
			queueSize int
		}
	}{
		{
			name:      "should create pool with specified parameters",
			workers:   5,
			queueSize: 100,
			want: struct {
				workers   int
				queueSize int
			}{
				workers:   5,
				queueSize: 100,
			},
		},
		{
			name:      "should handle single worker",
			workers:   1,
			queueSize: 10,
			want: struct {
				workers   int
				queueSize int
			}{
				workers:   1,
				queueSize: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			pool := NewFeedWorkerPool(tt.workers, tt.queueSize, logger)

			require.NotNil(t, pool)
			assert.Equal(t, tt.want.workers, pool.workers)
			assert.Equal(t, tt.want.queueSize, cap(pool.jobQueue))
		})
	}
}

func TestFeedWorkerPool_ProcessFeeds(t *testing.T) {
	tests := []struct {
		name     string
		feeds    []FeedJob
		fetcher  *MockArticleFetcher
		want     struct {
			successCount int
			errorCount   int
		}
	}{
		{
			name: "should process all feeds successfully",
			feeds: []FeedJob{
				{URL: "https://example.com/feed1"},
				{URL: "https://example.com/feed2"},
				{URL: "https://example.com/feed3"},
			},
			fetcher: &MockArticleFetcher{shouldFail: false},
			want: struct {
				successCount int
				errorCount   int
			}{
				successCount: 3,
				errorCount:   0,
			},
		},
		{
			name: "should handle mixed success and failure",
			feeds: []FeedJob{
				{URL: "https://example.com/feed1"},
				{URL: "https://example.com/feed2"},
			},
			fetcher: &MockArticleFetcher{shouldFail: true},
			want: struct {
				successCount int
				errorCount   int
			}{
				successCount: 0,
				errorCount:   2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			pool := NewFeedWorkerPool(2, 10, logger)

			ctx := context.Background()
			results := pool.ProcessFeeds(ctx, tt.feeds, tt.fetcher)

			successCount := 0
			errorCount := 0
			
			for _, result := range results {
				if result.Error != nil {
					errorCount++
				} else {
					successCount++
				}
			}

			assert.Equal(t, tt.want.successCount, successCount)
			assert.Equal(t, tt.want.errorCount, errorCount)
			assert.Equal(t, len(tt.feeds), len(results))
		})
	}
}

func TestFeedWorkerPool_ParallelExecution(t *testing.T) {
	t.Run("should execute feeds in parallel", func(t *testing.T) {
		fetchDelay := 100 * time.Millisecond
		fetcher := &MockArticleFetcher{
			fetchDelay: fetchDelay,
			shouldFail: false,
		}

		feeds := []FeedJob{
			{URL: "https://example.com/feed1"},
			{URL: "https://example.com/feed2"},
			{URL: "https://example.com/feed3"},
			{URL: "https://example.com/feed4"},
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		pool := NewFeedWorkerPool(4, 10, logger)

		start := time.Now()
		ctx := context.Background()
		results := pool.ProcessFeeds(ctx, feeds, fetcher)
		elapsed := time.Since(start)

		// With 4 workers and 4 feeds, it should take roughly fetchDelay time
		// (plus some overhead), not 4 * fetchDelay
		expectedMaxTime := fetchDelay + 200*time.Millisecond
		assert.Less(t, elapsed, expectedMaxTime)
		assert.Equal(t, 4, len(results))
		
		// Verify all fetches were made
		assert.Equal(t, 4, fetcher.fetchCount)
	})
}

func TestFeedWorkerPool_ContextCancellation(t *testing.T) {
	t.Run("should respect context cancellation", func(t *testing.T) {
		fetcher := &MockArticleFetcher{
			fetchDelay: 500 * time.Millisecond,
			shouldFail: false,
		}

		feeds := []FeedJob{
			{URL: "https://example.com/feed1"},
			{URL: "https://example.com/feed2"},
		}

		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		pool := NewFeedWorkerPool(2, 10, logger)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		results := pool.ProcessFeeds(ctx, feeds, fetcher)
		elapsed := time.Since(start)

		// Should return quickly due to context timeout
		assert.Less(t, elapsed, 300*time.Millisecond)
		
		// Some results may be empty due to cancellation
		assert.LessOrEqual(t, len(results), len(feeds))
	})
}