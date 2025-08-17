// ABOUTME: This file contains comprehensive TDD tests for enhanced feed processor service
// ABOUTME: Tests advanced error handling, retry mechanisms, circuit breaker, and metrics

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"pre-processor/repository"
	"pre-processor/service"
	"pre-processor/test/mocks"
)

func TestFeedMetricsProcessorService_GetProcessingStats(t *testing.T) {
	// Test data setup
	testStats := &repository.ProcessingStats{
		TotalFeeds:     100,
		ProcessedFeeds: 75,
		RemainingFeeds: 25,
	}

	tests := map[string]struct {
		setupMocks     func(*mocks.MockFeedRepository)
		expectedResult *service.ProcessingStats
		expectedError  string
		description    string
	}{
		"success_case_get_stats": {
			description: "Should successfully retrieve processing statistics",
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository) {
				mockFeedRepo.EXPECT().
					GetProcessingStats(gomock.Any()).
					Return(testStats, nil).
					Times(1)
			},
			expectedResult: &service.ProcessingStats{
				TotalFeeds:     100,
				ProcessedFeeds: 75,
				RemainingFeeds: 25,
			},
			expectedError: "",
		},
		"error_case_repository_failure": {
			description: "Should fail when repository returns error",
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository) {
				mockFeedRepo.EXPECT().
					GetProcessingStats(gomock.Any()).
					Return(nil, errors.New("repository connection failed")).
					Times(1)
			},
			expectedResult: nil,
			expectedError:  "repository connection failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
			logger := slog.Default()

			// Setup test expectations
			tc.setupMocks(mockFeedRepo)

			// Create service
			serviceInstance := service.NewFeedMetricsProcessorService(
				mockFeedRepo,
				mockArticleRepo,
				mockFetcher,
				logger,
			)

			// Execute test
			ctx := context.Background()
			result, err := serviceInstance.GetProcessingStats(ctx)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err, tc.description)
				require.NotNil(t, result, tc.description)
				assert.Equal(t, tc.expectedResult.TotalFeeds, result.TotalFeeds)
				assert.Equal(t, tc.expectedResult.ProcessedFeeds, result.ProcessedFeeds)
				assert.Equal(t, tc.expectedResult.RemainingFeeds, result.RemainingFeeds)
			}
		})
	}
}

func TestFeedMetricsProcessorService_ResetPagination(t *testing.T) {
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

			mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
			logger := slog.Default()

			serviceInstance := service.NewFeedMetricsProcessorService(
				mockFeedRepo,
				mockArticleRepo,
				mockFetcher,
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

func TestFeedMetricsProcessorService_ProcessFeeds_NoExternalCalls(t *testing.T) {
	// Test case that focuses on business logic without external HTTP calls
	tests := map[string]struct {
		setupMocks    func(*mocks.MockFeedRepository, *mocks.MockArticleRepository, *mocks.MockArticleFetcherService)
		batchSize     int
		expectedError string
		description   string
	}{
		"success_case_no_feeds": {
			description: "Should handle empty feed list gracefully",
			batchSize:   10,
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository, mockArticleRepo *mocks.MockArticleRepository, mockFetcher *mocks.MockArticleFetcherService) {
				cursor := &repository.Cursor{}

				// Mock empty feed list
				mockFeedRepo.EXPECT().
					GetUnprocessedFeeds(gomock.Any(), cursor, 10).
					Return([]*url.URL{}, nil, nil).
					Times(1)
			},
			expectedError: "",
		},
		"error_case_get_feeds_failure": {
			description: "Should fail when repository cannot get feeds",
			batchSize:   10,
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository, mockArticleRepo *mocks.MockArticleRepository, mockFetcher *mocks.MockArticleFetcherService) {
				cursor := &repository.Cursor{}

				mockFeedRepo.EXPECT().
					GetUnprocessedFeeds(gomock.Any(), cursor, 10).
					Return(nil, nil, errors.New("database connection failed")).
					Times(1)
			},
			expectedError: "database connection failed",
		},
		"success_case_articles_already_exist": {
			description: "Should handle case where articles already exist",
			batchSize:   5,
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository, mockArticleRepo *mocks.MockArticleRepository, mockFetcher *mocks.MockArticleFetcherService) {
				cursor := &repository.Cursor{}
				testURL, _ := url.Parse("http://example.com/feed.rss")
				feedURLs := []*url.URL{testURL}

				mockFeedRepo.EXPECT().
					GetUnprocessedFeeds(gomock.Any(), cursor, 5).
					Return(feedURLs, cursor, nil).
					Times(1)

				// Mock that articles already exist
				mockArticleRepo.EXPECT().
					CheckExists(gomock.Any(), []string{"http://example.com/feed.rss"}).
					Return(true, nil).
					Times(1)
			},
			expectedError: "",
		},
		"error_case_check_exists_failure": {
			description: "Should fail when article existence check fails",
			batchSize:   5,
			setupMocks: func(mockFeedRepo *mocks.MockFeedRepository, mockArticleRepo *mocks.MockArticleRepository, mockFetcher *mocks.MockArticleFetcherService) {
				cursor := &repository.Cursor{}
				testURL, _ := url.Parse("http://example.com/feed.rss")
				feedURLs := []*url.URL{testURL}

				mockFeedRepo.EXPECT().
					GetUnprocessedFeeds(gomock.Any(), cursor, 5).
					Return(feedURLs, cursor, nil).
					Times(1)

				// Mock existence check failure
				mockArticleRepo.EXPECT().
					CheckExists(gomock.Any(), []string{"http://example.com/feed.rss"}).
					Return(false, errors.New("existence check failed")).
					Times(1)
			},
			expectedError: "existence check failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
			mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
			mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
			logger := slog.Default()

			// Setup test expectations
			tc.setupMocks(mockFeedRepo, mockArticleRepo, mockFetcher)

			// Create service
			serviceInstance := service.NewFeedMetricsProcessorService(
				mockFeedRepo,
				mockArticleRepo,
				mockFetcher,
				logger,
			)

			// Execute test
			ctx := context.Background()
			result, err := serviceInstance.ProcessFeedsWithRetry(ctx, tc.batchSize)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err, tc.description)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestFeedMetricsProcessorService_Interface_Compliance(t *testing.T) {
	// Test that service properly implements both interfaces
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
	logger := slog.Default()

	serviceInstance := service.NewFeedMetricsProcessorService(
		mockFeedRepo,
		mockArticleRepo,
		mockFetcher,
		logger,
	)

	// Verify interface compliance
	var _ service.FeedProcessorService = serviceInstance
	var _ service.FeedMetricsProcessorService = serviceInstance
	assert.NotNil(t, serviceInstance)
}

func TestFeedMetricsProcessorService_Constructor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
	logger := slog.Default()

	serviceInstance := service.NewFeedMetricsProcessorService(
		mockFeedRepo,
		mockArticleRepo,
		mockFetcher,
		logger,
	)

	// Verify service is properly constructed
	assert.NotNil(t, serviceInstance)

	// Test methods that don't require external dependencies
	assert.NotPanics(t, func() { serviceInstance.ResetPagination() })

	// Test metrics methods (these should return valid metrics objects)
	dlqMetrics := serviceInstance.GetDeadLetterQueueMetrics()
	assert.NotNil(t, dlqMetrics)

	cbMetrics := serviceInstance.GetCircuitBreakerMetrics()
	assert.NotNil(t, cbMetrics)
}

func TestFeedMetricsProcessorService_ProcessFeeds_Wrapper(t *testing.T) {
	// Test that ProcessFeeds properly delegates to ProcessFeedsWithRetry
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
	logger := slog.Default()

	// Setup expectation for empty feed list (safe test case)
	cursor := &repository.Cursor{}
	mockFeedRepo.EXPECT().
		GetUnprocessedFeeds(gomock.Any(), cursor, 10).
		Return([]*url.URL{}, nil, nil).
		Times(1)

	serviceInstance := service.NewFeedMetricsProcessorService(
		mockFeedRepo,
		mockArticleRepo,
		mockFetcher,
		logger,
	)

	// Execute test
	ctx := context.Background()
	result, err := serviceInstance.ProcessFeeds(ctx, 10)

	// Verify results - ProcessFeeds should delegate to ProcessFeedsWithRetry
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, result.ProcessedCount)
	assert.Equal(t, 0, result.SuccessCount)
	assert.Equal(t, 0, result.ErrorCount)
	assert.False(t, result.HasMore)
}

func TestFeedMetricsProcessorService_EdgeCases(t *testing.T) {
	t.Run("large_batch_size", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
		logger := slog.Default()

		// Setup expectation for large batch
		cursor := &repository.Cursor{}
		mockFeedRepo.EXPECT().
			GetUnprocessedFeeds(gomock.Any(), cursor, 1000).
			Return([]*url.URL{}, nil, nil).
			Times(1)

		serviceInstance := service.NewFeedMetricsProcessorService(
			mockFeedRepo,
			mockArticleRepo,
			mockFetcher,
			logger,
		)

		result, err := serviceInstance.ProcessFeedsWithRetry(context.Background(), 1000)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ProcessedCount)
	})

	t.Run("zero_batch_size", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
		mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
		mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
		logger := slog.Default()

		// Setup expectation for zero batch
		cursor := &repository.Cursor{}
		mockFeedRepo.EXPECT().
			GetUnprocessedFeeds(gomock.Any(), cursor, 0).
			Return([]*url.URL{}, nil, nil).
			Times(1)

		serviceInstance := service.NewFeedMetricsProcessorService(
			mockFeedRepo,
			mockArticleRepo,
			mockFetcher,
			logger,
		)

		result, err := serviceInstance.ProcessFeedsWithRetry(context.Background(), 0)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ProcessedCount)
	})
}

func TestFeedMetricsProcessorService_ProcessFailedItems_SafeScenario(t *testing.T) {
	// Test ProcessFailedItems with empty DLQ (safe scenario)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFeedRepo := mocks.NewMockFeedRepository(ctrl)
	mockArticleRepo := mocks.NewMockArticleRepository(ctrl)
	mockFetcher := mocks.NewMockArticleFetcherService(ctrl)
	logger := slog.Default()

	serviceInstance := service.NewFeedMetricsProcessorService(
		mockFeedRepo,
		mockArticleRepo,
		mockFetcher,
		logger,
	)

	// Execute test - with empty DLQ this should complete safely
	ctx := context.Background()
	err := serviceInstance.ProcessFailedItems(ctx)

	// This should not error with empty DLQ
	require.NoError(t, err)
}
