package service_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/service"
	"pre-processor/test/mocks"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func testQueueGuardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

func TestShouldQueueSummarizeJob(t *testing.T) {
	t.Run("skips when summary already exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-1").Return(true, nil)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-1", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.False(t, shouldQueue)
		require.Equal(t, "summary_exists", reason)
	})

	t.Run("skips when recent successful job exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-2").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-2", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, since time.Time) (bool, error) {
				require.WithinDuration(t, time.Now().Add(-24*time.Hour), since, 2*time.Second)
				return true, nil
			},
		)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-2", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.False(t, shouldQueue)
		require.Equal(t, "recent_success", reason)
	})

	t.Run("allows queue creation when no existing summary or recent success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-3").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-3", gomock.Any()).Return(false, nil)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-3", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.True(t, shouldQueue)
		require.Empty(t, reason)
	})
}
