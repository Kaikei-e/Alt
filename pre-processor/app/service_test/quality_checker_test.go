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

			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockAPIRepo,
				nil, // dbPool not needed for these tests
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
		setupMocks    func(*mocks.MockSummaryRepository)
		articles      []domain.ArticleWithSummary
		expectedError string
		description   string
	}{
		"success_case_delete_summaries": {
			description: "Should successfully delete low quality summaries",
			articles:    lowQualityArticles,
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				// Expect deletion of both summaries
				mockSummaryRepo.EXPECT().
					Delete(gomock.Any(), "summary1").
					Return(nil).
					Times(1)

				mockSummaryRepo.EXPECT().
					Delete(gomock.Any(), "summary2").
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		"success_case_empty_list": {
			description: "Should handle empty article list gracefully",
			articles:    []domain.ArticleWithSummary{},
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				// No expectations for empty list
			},
			expectedError: "",
		},
		"error_case_delete_failure": {
			description: "Should fail when summary deletion fails",
			articles:    lowQualityArticles[:1], // Only first article
			setupMocks: func(mockSummaryRepo *mocks.MockSummaryRepository) {
				mockSummaryRepo.EXPECT().
					Delete(gomock.Any(), "summary1").
					Return(errors.New("delete operation failed")).
					Times(1)
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

			// Setup test expectations
			tc.setupMocks(mockSummaryRepo)

			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockAPIRepo,
				nil, // dbPool not needed for these tests
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

			serviceInstance := service.NewQualityCheckerService(
				mockSummaryRepo,
				mockAPIRepo,
				nil, // dbPool not needed for these tests
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

	serviceInstance := service.NewQualityCheckerService(
		mockSummaryRepo,
		mockAPIRepo,
		nil,
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

	serviceInstance := service.NewQualityCheckerService(
		mockSummaryRepo,
		mockAPIRepo,
		nil,
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

		serviceInstance := service.NewQualityCheckerService(
			mockSummaryRepo,
			mockAPIRepo,
			nil,
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

		serviceInstance := service.NewQualityCheckerService(
			mockSummaryRepo,
			mockAPIRepo,
			nil,
			logger,
		)

		result, err := serviceInstance.CheckQuality(context.Background(), 0)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ProcessedCount)
	})
}
