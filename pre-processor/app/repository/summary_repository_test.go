// ABOUTME: This file contains comprehensive tests for the Summary Repository
// ABOUTME: It follows TDD principles with table-driven tests for all methods

package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLoggerSummaryRepo() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestSummaryRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement SummaryRepository interface", func(t *testing.T) {
		// RED PHASE: Test that repository implements interface
		repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

		// Verify interface compliance at compile time
		var _ SummaryRepository = repo

		assert.NotNil(t, repo)
	})
}

func TestSummaryRepository_Create(t *testing.T) {
	tests := map[string]struct {
		summary     *models.ArticleSummary
		errContains string
		setupLogger bool
		wantErr     bool
	}{
		"should handle nil database gracefully": {
			summary: &models.ArticleSummary{
				ArticleID:       "test-article-123",
				ArticleTitle:    "Test Article",
				SummaryJapanese: "テスト記事の要約",
			},
			
			wantErr:     true,
			errContains: "failed to create article summary",
		},
		"should handle nil summary": {
			summary:     nil,
			
			wantErr:     true,
			errContains: "summary cannot be nil",
		},
		"should handle empty article ID": {
			summary: &models.ArticleSummary{
				ArticleID:       "",
				ArticleTitle:    "Test Article",
				SummaryJapanese: "テスト記事の要約",
			},
			
			wantErr:     true,
			errContains: "article ID cannot be empty",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

			err := repo.Create(context.Background(), tc.summary)

			if tc.wantErr {
				require.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSummaryRepository_FindArticlesWithSummaries(t *testing.T) {
	tests := map[string]struct {
		cursor      *Cursor
		errContains string
		limit       int
		setupLogger bool
		wantErr     bool
	}{
		"should handle nil database gracefully": {
			cursor:      nil,
			limit:       10,
			
			wantErr:     true,
			errContains: "failed to find articles with summaries",
		},
		"should handle with cursor": {
			cursor: &Cursor{
				LastCreatedAt: &time.Time{},
				LastID:        "last-123",
			},
			limit:       5,
			
			wantErr:     true,
			errContains: "failed to find articles with summaries",
		},
		"should handle zero limit": {
			cursor:      nil,
			limit:       0,
			
			wantErr:     true,
			errContains: "limit must be positive",
		},
		"should handle negative limit": {
			cursor:      nil,
			limit:       -1,
			
			wantErr:     true,
			errContains: "limit must be positive",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

			articles, cursor, err := repo.FindArticlesWithSummaries(context.Background(), tc.cursor, tc.limit)

			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, articles)
				assert.Nil(t, cursor)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, articles)
				assert.NotNil(t, cursor)
			}
		})
	}
}

func TestSummaryRepository_Delete(t *testing.T) {
	tests := map[string]struct {
		summaryID   string
		errContains string
		setupLogger bool
		wantErr     bool
	}{
		"should handle nil database gracefully": {
			summaryID:   "summary-123",
			
			wantErr:     true,
			errContains: "failed to delete article summary",
		},
		"should handle empty summary ID": {
			summaryID:   "",
			
			wantErr:     true,
			errContains: "summary ID cannot be empty",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

			err := repo.Delete(context.Background(), tc.summaryID)

			if tc.wantErr {
				require.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSummaryRepository_Exists(t *testing.T) {
	tests := map[string]struct {
		summaryID   string
		errContains string
		setupLogger bool
		wantErr     bool
		wantExists  bool
	}{
		"should handle nil database gracefully": {
			summaryID:   "summary-123",
			
			wantErr:     true,
			wantExists:  false,
			errContains: "failed to check if article summary exists",
		},
		"should handle empty summary ID": {
			summaryID:   "",
			
			wantErr:     true,
			wantExists:  false,
			errContains: "summary ID cannot be empty",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GREEN PHASE: Test minimal implementation

			repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

			exists, err := repo.Exists(context.Background(), tc.summaryID)

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tc.wantExists, exists)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantExists, exists)
			}
		})
	}
}

func TestSummaryRepository_ErrorHandling(t *testing.T) {
	t.Run("should handle context cancellation", func(t *testing.T) {
		

		repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel context immediately

		// Test Create
		err := repo.Create(ctx, &models.ArticleSummary{
			ArticleID: "test-123",
		})
		assert.Error(t, err)

		// Test FindArticlesWithSummaries
		articles, cursor, err := repo.FindArticlesWithSummaries(ctx, nil, 10)
		assert.Error(t, err)
		assert.Nil(t, articles)
		assert.Nil(t, cursor)

		// Test Delete
		err = repo.Delete(ctx, "summary-123")
		assert.Error(t, err)

		// Test Exists
		exists, err := repo.Exists(ctx, "summary-123")
		assert.Error(t, err)
		assert.False(t, exists)
	})
}

func TestSummaryRepository_EdgeCases(t *testing.T) {
	t.Run("should handle large limit values", func(t *testing.T) {
		

		repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

		// Test with very large limit
		articles, cursor, err := repo.FindArticlesWithSummaries(context.Background(), nil, 1000000)

		assert.Error(t, err)
		assert.Nil(t, articles)
		assert.Nil(t, cursor)
	})

	t.Run("should handle cursor with nil LastCreatedAt", func(t *testing.T) {
		

		repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

		cursor := &Cursor{
			LastCreatedAt: nil,
			LastID:        "test-123",
		}

		articles, newCursor, err := repo.FindArticlesWithSummaries(context.Background(), cursor, 10)

		assert.Error(t, err)
		assert.Nil(t, articles)
		assert.Nil(t, newCursor)
	})

	t.Run("should handle cursor with empty LastID", func(t *testing.T) {
		

		repo := NewSummaryRepository(nil, testLoggerSummaryRepo())

		now := time.Now()
		cursor := &Cursor{
			LastCreatedAt: &now,
			LastID:        "",
		}

		articles, newCursor, err := repo.FindArticlesWithSummaries(context.Background(), cursor, 10)

		assert.Error(t, err)
		assert.Nil(t, articles)
		assert.Nil(t, newCursor)
	})
}

// Table-driven tests for comprehensive coverage.
func TestSummaryRepository_TableDriven(t *testing.T) {
	type testCase struct {
		setup       func() (SummaryRepository, interface{})
		validate    func(t *testing.T, result interface{}, err error)
		name        string
		operation   string
		setupLogger bool
	}

	tests := []testCase{
		{
			name:      "create with all fields populated",
			operation: "create",
			setup: func() (SummaryRepository, interface{}) {
				repo := NewSummaryRepository(nil, testLoggerSummaryRepo())
				summary := &models.ArticleSummary{
					ArticleID:       "article-456",
					ArticleTitle:    "Test Article Title",
					SummaryJapanese: "これはテスト記事の要約です",
				}
				return repo, summary
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to create article summary")
			},
			
		},
		{
			name:      "find with valid parameters",
			operation: "find",
			setup: func() (SummaryRepository, interface{}) {
				repo := NewSummaryRepository(nil, testLoggerSummaryRepo())
				params := struct {
					cursor *Cursor
					limit  int
				}{
					cursor: nil,
					limit:  20,
				}
				return repo, params
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to find articles with summaries")
			},
			
		},
		{
			name:      "delete with valid ID",
			operation: "delete",
			setup: func() (SummaryRepository, interface{}) {
				repo := NewSummaryRepository(nil, testLoggerSummaryRepo())
				return repo, "summary-789"
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to delete article summary")
			},
			
		},
		{
			name:      "exists with valid ID",
			operation: "exists",
			setup: func() (SummaryRepository, interface{}) {
				repo := NewSummaryRepository(nil, testLoggerSummaryRepo())
				return repo, "summary-101"
			},
			validate: func(t *testing.T, result interface{}, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to check if article summary exists")
			},
			
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			repo, input := tc.setup()
			ctx := context.Background()

			var result interface{}

			var err error

			switch tc.operation {
			case "create":
				err = repo.Create(ctx, input.(*models.ArticleSummary))
			case "find":
				params := input.(struct {
					cursor *Cursor
					limit  int
				})

				var articles []*models.ArticleWithSummary

				var cursor *Cursor
				articles, cursor, err = repo.FindArticlesWithSummaries(ctx, params.cursor, params.limit)
				result = struct {
					cursor   *Cursor
					articles []*models.ArticleWithSummary
				}{cursor, articles}
			case "delete":
				err = repo.Delete(ctx, input.(string))
			case "exists":
				var exists bool
				exists, err = repo.Exists(ctx, input.(string))
				result = exists
			}

			tc.validate(t, result, err)
		})
	}
}
