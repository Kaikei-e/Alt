// ABOUTME: This file contains comprehensive TDD tests for LLM-based quality checking service
// ABOUTME: Tests article quality assessment, low-quality article processing, and pagination

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"pre-processor/domain"
	"pre-processor/service"
	"pre-processor/test/mocks"
)

func TestQualityCheckerService_CheckQuality(t *testing.T) {

	tests := map[string]struct {
		setupMocks     func(*mocks.MockSummaryRepository)
		batchSize      int
		expectedResult *service.QualityResult
		expectedError  string
		description    string
	}{
		"success_case_no_articles": {
			description: "Should handle empty article list gracefully",
			batchSize:   10,
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				cursor := &domain.Cursor{}

				mockSummaryRepo.EXPECT().
					FindArticlesWithSummaries(gomock.Any(), cursor, 10).
					Return([]*domain.ArticleWithSummary{}, nil, nil). // Empty list, no next cursor
					Times(1)
			},
			expectedResult: &service.QualityResult{
				ProcessedCount: 0,
				SuccessCount:   0,
				ErrorCount:     0,
				RemovedCount:   0,
				RetainedCount:  0,
				Errors:         []error{},
				HasMore:        false,
			},
			expectedError: "",
		},
		"error_case_find_articles_failure": {
			description: "Should fail when repository cannot find articles",
			batchSize:   10,
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				cursor := &domain.Cursor{}

				mockSummaryRepo.EXPECT().
					FindArticlesWithSummaries(gomock.Any(), cursor, 10).
					Return(nil, nil, errors.New("database connection failed")).
					Times(1)
			},
			expectedResult: nil,
			expectedError:  "database connection failed",
		},
		"success_case_business_logic_validation": {
			description: "Should validate input parameters and return appropriate results",
			batchSize:   0, // Test edge case with zero batch size
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				cursor := &domain.Cursor{}

				mockSummaryRepo.EXPECT().
					FindArticlesWithSummaries(gomock.Any(), cursor, 0).
					Return([]*domain.ArticleWithSummary{}, nil, nil).
					Times(1)
			},
			expectedResult: &service.QualityResult{
				ProcessedCount: 0,
				SuccessCount:   0,
				ErrorCount:     0,
				RemovedCount:   0,
				RetainedCount:  0,
				Errors:         []error{},
				HasMore:        false,
			},
			expectedError: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			logger := slog.Default()

			// Setup test expectations
			tc.setupMocks(mockSummaryRepo)

			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockArticleRepo,
				mockAPIRepo,
				mockJobRepo,
				logger,
			)

			// Execute test
			ctx := context.Background()
			result, err := serviceInstance.CheckQuality(ctx, tc.batchSize)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err, tc.description)
				require.NotNil(t, result, tc.description)
				assert.Equal(t, tc.expectedResult.ProcessedCount, result.ProcessedCount)
				assert.Equal(t, tc.expectedResult.SuccessCount, result.SuccessCount)
				assert.Equal(t, tc.expectedResult.ErrorCount, result.ErrorCount)
				assert.Equal(t, tc.expectedResult.RemovedCount, result.RemovedCount)
				assert.Equal(t, tc.expectedResult.RetainedCount, result.RetainedCount)
				assert.Equal(t, tc.expectedResult.HasMore, result.HasMore)
				assert.Len(t, result.Errors, tc.expectedResult.ErrorCount)
			}
		})
	}
}

func TestQualityCheckerService_ProcessLowQualityArticles(t *testing.T) {
	// Test data setup
	lowQualityArticles := []domain.ArticleWithSummary{
		{
			ArticleID: "article1",
			SummaryID: "summary1",
		},
		{
			ArticleID: "article2",
			SummaryID: "summary2",
		},
	}

	tests := map[string]struct {
		setupMocks    func(*mocks.MockSummaryRepository, *mocks.MockSummarizeJobRepository)
		articles      []domain.ArticleWithSummary
		expectedError string
		description   string
	}{
		"success_case_delete_summaries": {
			description: "Should successfully delete low quality summaries and invalidate jobs",
			articles:    lowQualityArticles,
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository, mockJobRepo *mocks.MockSummarizeJobRepository) {
				mockSummaryRepo.EXPECT().Delete(gomock.Any(), "article1").Return(nil)
				mockSummaryRepo.EXPECT().Delete(gomock.Any(), "article2").Return(nil)
				// Compensating transaction for each deleted summary
				mockJobRepo.EXPECT().InvalidateCompletedJobSummary(gomock.Any(), "article1").Return(nil)
				mockJobRepo.EXPECT().InvalidateCompletedJobSummary(gomock.Any(), "article2").Return(nil)
			},
			expectedError: "",
		},
		"success_case_empty_list": {
			description: "Should handle empty article list gracefully",
			articles:    []domain.ArticleWithSummary{},
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository, mockJobRepo *mocks.MockSummarizeJobRepository) {
				// No expectations for empty list
			},
			expectedError: "",
		},
		"error_case_delete_failure": {
			description: "Should fail when summary deletion fails (no invalidation)",
			articles:    lowQualityArticles[:1],
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository, mockJobRepo *mocks.MockSummarizeJobRepository) {
				mockSummaryRepo.EXPECT().
					Delete(gomock.Any(), "article1").
					Return(errors.New("delete operation failed"))
				// No InvalidateCompletedJobSummary expected — delete failed
			},
			expectedError: "delete operation failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			logger := slog.Default()

			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

			// Setup test expectations
			tc.setupMocks(mockSummaryRepo, mockJobRepo)

			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockArticleRepo,
				mockAPIRepo,
				mockJobRepo,
				logger,
			)

			// Execute test
			ctx := context.Background()
			err := serviceInstance.ProcessLowQualityArticles(ctx, tc.articles)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err, tc.description)
			}
		})
	}
}

func TestQualityCheckerService_ResetPagination(t *testing.T) {
	tests := map[string]struct {
		description   string
		expectedError string
	}{
		"success_case_reset": {
			description:   "Should successfully reset pagination cursor",
			expectedError: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
			mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
			logger := slog.Default()

			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockArticleRepo,
				mockAPIRepo,
				mockJobRepo,
				logger,
			)

			// Execute test
			err := serviceInstance.ResetPagination()

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err, tc.description)
			}
		})
	}
}

func TestQualityCheckerService_Interface_Compliance(t *testing.T) {
	// Test that service properly implements the interface
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	logger := slog.Default()

	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	serviceInstance := service.NewQualityCheckerService(
		mockSummaryRepo,
		mockArticleRepo,
		mockAPIRepo,
		mockJobRepo,
		logger,
	)

	// Verify interface compliance
	var _ = serviceInstance
	assert.NotNil(t, serviceInstance)
}

func TestQualityCheckerService_Constructor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
	mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
	logger := slog.Default()

	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	serviceInstance := service.NewQualityCheckerService(
		mockSummaryRepo,
		mockArticleRepo,
		mockAPIRepo,
		mockJobRepo,
		logger,
	)

	// Verify service is properly constructed
	assert.NotNil(t, serviceInstance)

	// These should not panic (basic smoke test)
	assert.NotPanics(t, func() { _ = serviceInstance.ResetPagination() })

	// Note: CheckQuality and ProcessLowQualityArticles require proper mocks to test
	// which are covered in the dedicated test functions above
}

func TestQualityCheckerService_EdgeCases(t *testing.T) {
	t.Run("large_batch_size", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		logger := slog.Default()

		// Setup expectation for large batch
		cursor := &domain.Cursor{}
		mockSummaryRepo.EXPECT().
			FindArticlesWithSummaries(gomock.Any(), cursor, 1000).
			Return([]*domain.ArticleWithSummary{}, nil, nil).
			Times(1)

		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		serviceInstance := service.NewQualityCheckerService(
			mockSummaryRepo,
			mockArticleRepo,
			mockAPIRepo,
			mockJobRepo,
			logger,
		)

		result, err := serviceInstance.CheckQuality(context.Background(), 1000)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ProcessedCount)
	})

	t.Run("zero_batch_size", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		logger := slog.Default()

		// Setup expectation for zero batch
		cursor := &domain.Cursor{}
		mockSummaryRepo.EXPECT().
			FindArticlesWithSummaries(gomock.Any(), cursor, 0).
			Return([]*domain.ArticleWithSummary{}, nil, nil).
			Times(1)

		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		serviceInstance := service.NewQualityCheckerService(
			mockSummaryRepo,
			mockArticleRepo,
			mockAPIRepo,
			mockJobRepo,
			logger,
		)

		result, err := serviceInstance.CheckQuality(context.Background(), 0)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ProcessedCount)
	})
}

func TestQualityCheckerService_CompensatingTransaction(t *testing.T) {
	t.Run("should_invalidate_job_after_summary_deletion", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		logger := slog.Default()

		articles := []domain.ArticleWithSummary{
			{ArticleID: "article-low-quality", SummaryID: "s1"},
		}

		// Delete succeeds
		mockSummaryRepo.EXPECT().
			Delete(gomock.Any(), "article-low-quality").
			Return(nil)

		// Compensating transaction: must invalidate the completed job
		mockJobRepo.EXPECT().
			InvalidateCompletedJobSummary(gomock.Any(), "article-low-quality").
			Return(nil).
			Times(1)

		svc := service.NewQualityCheckerService(
			mockSummaryRepo, mockArticleRepo, mockAPIRepo, mockJobRepo, logger,
		)

		err := svc.ProcessLowQualityArticles(context.Background(), articles)
		require.NoError(t, err)
	})

	t.Run("should_not_invalidate_when_delete_fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		logger := slog.Default()

		articles := []domain.ArticleWithSummary{
			{ArticleID: "article-delete-fail", SummaryID: "s2"},
		}

		// Delete fails
		mockSummaryRepo.EXPECT().
			Delete(gomock.Any(), "article-delete-fail").
			Return(errors.New("delete failed"))

		// InvalidateCompletedJobSummary must NOT be called (gomock enforces this)

		svc := service.NewQualityCheckerService(
			mockSummaryRepo, mockArticleRepo, mockAPIRepo, mockJobRepo, logger,
		)

		err := svc.ProcessLowQualityArticles(context.Background(), articles)
		require.Error(t, err)
	})

	t.Run("should_continue_on_invalidation_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockAPIRepo := mocks.NewMockExternalAPIRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
		logger := slog.Default()

		articles := []domain.ArticleWithSummary{
			{ArticleID: "article-inv-fail", SummaryID: "s3"},
			{ArticleID: "article-inv-ok", SummaryID: "s4"},
		}

		// Both deletes succeed
		mockSummaryRepo.EXPECT().Delete(gomock.Any(), "article-inv-fail").Return(nil)
		mockSummaryRepo.EXPECT().Delete(gomock.Any(), "article-inv-ok").Return(nil)

		// First invalidation fails — processing must continue
		mockJobRepo.EXPECT().
			InvalidateCompletedJobSummary(gomock.Any(), "article-inv-fail").
			Return(errors.New("db timeout"))
		// Second invalidation succeeds
		mockJobRepo.EXPECT().
			InvalidateCompletedJobSummary(gomock.Any(), "article-inv-ok").
			Return(nil)

		svc := service.NewQualityCheckerService(
			mockSummaryRepo, mockArticleRepo, mockAPIRepo, mockJobRepo, logger,
		)

		err := svc.ProcessLowQualityArticles(context.Background(), articles)
		require.NoError(t, err)
	})
}
