package driver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/models"
)

func TestBatchInsertArticles(t *testing.T) {
	tests := []struct {
		name     string
		articles []models.Article
		want     struct {
			shouldSucceed bool
			errorContains string
		}
	}{
		{
			name: "should insert multiple articles in batch",
			articles: []models.Article{
				{
					Title:   "Article 1",
					Content: "Content 1",
					URL:     "https://example.com/1",
					FeedID:  "1",
				},
				{
					Title:   "Article 2",
					Content: "Content 2",
					URL:     "https://example.com/2",
					FeedID:  "1",
				},
				{
					Title:   "Article 3",
					Content: "Content 3",
					URL:     "https://example.com/3",
					FeedID:  "2",
				},
			},
			want: struct {
				shouldSucceed bool
				errorContains string
			}{
				shouldSucceed: true,
			},
		},
		{
			name:     "should handle empty articles slice",
			articles: []models.Article{},
			want: struct {
				shouldSucceed bool
				errorContains string
			}{
				shouldSucceed: true,
			},
		},
		{
			name: "should handle single article",
			articles: []models.Article{
				{
					Title:   "Single Article",
					Content: "Single Content",
					URL:     "https://example.com/single",
					FeedID:  "1",
				},
			},
			want: struct {
				shouldSucceed bool
				errorContains string
			}{
				shouldSucceed: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database interface
			db := &MockDB{}

			ctx := context.Background()
			err := BatchInsertArticles(ctx, db, tt.articles)

			if tt.want.shouldSucceed {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.want.errorContains)
			}
		})
	}
}

func TestBatchUpdateArticles(t *testing.T) {
	tests := []struct {
		name     string
		articles []models.Article
		want     struct {
			shouldSucceed bool
		}
	}{
		{
			name: "should update multiple articles in batch",
			articles: []models.Article{
				{
					ID:      "1",
					Title:   "Updated Article 1",
					Content: "Updated Content 1",
					URL:     "https://example.com/1",
				},
				{
					ID:      "2",
					Title:   "Updated Article 2",
					Content: "Updated Content 2",
					URL:     "https://example.com/2",
				},
			},
			want: struct {
				shouldSucceed bool
			}{
				shouldSucceed: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &MockDB{}

			ctx := context.Background()
			err := BatchUpdateArticles(ctx, db, tt.articles)

			if tt.want.shouldSucceed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestBatchOperations_Performance(t *testing.T) {
	t.Run("should handle batch operations efficiently", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping performance test")
		}

		// Generate test articles
		articles := make([]models.Article, 100)
		for i := 0; i < 100; i++ {
			articles[i] = models.Article{
				Title:   "Performance Test Article",
				Content: "Performance test content",
				URL:     "https://example.com/perf/" + string(rune(i)),
				FeedID:  "1",
			}
		}

		db := &MockDB{}
		ctx := context.Background()

		// Test batch insert performance
		start := time.Now()
		err := BatchInsertArticles(ctx, db, articles)
		batchDuration := time.Since(start)

		assert.NoError(t, err)

		// Batch operation should complete reasonably fast (< 100ms for mock)
		assert.Less(t, batchDuration.Milliseconds(), int64(100))

		// Test that batch operations can handle large numbers of articles
		largeArticles := make([]models.Article, 1000)
		for i := 0; i < 1000; i++ {
			largeArticles[i] = models.Article{
				Title:   "Large Test Article",
				Content: "Large test content",
				URL:     "https://example.com/large/" + string(rune(i)),
				FeedID:  "1",
			}
		}

		start = time.Now()
		err = BatchInsertArticles(ctx, db, largeArticles)
		largeBatchDuration := time.Since(start)

		assert.NoError(t, err)
		// Large batch should still complete in reasonable time (< 500ms for mock)
		assert.Less(t, largeBatchDuration.Milliseconds(), int64(500))
	})
}
