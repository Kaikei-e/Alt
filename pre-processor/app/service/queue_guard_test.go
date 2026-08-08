package service_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/service"
	"pre-processor/test/mocks"

	"github.com/stretchr/testify/assert"
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

	t.Run("skips when an in-flight job for the article exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-inflight").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-inflight", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-inflight", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, since time.Time) (bool, error) {
				require.WithinDuration(t, time.Now().Add(-10*time.Minute), since, 2*time.Second)
				return true, nil
			},
		)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-inflight", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.False(t, shouldQueue)
		require.Equal(t, "in_flight", reason)
	})

	t.Run("allows queue creation when no existing summary or recent success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-3").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-3", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-3", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "article-3").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), "article-3", gomock.Any()).Return(false, nil)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-3", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.True(t, shouldQueue)
		require.Empty(t, reason)
	})

	// DEFECT: an article whose model call the LLM can never process
	// (dead_letter = permanent failure per UpdateJobStatus) satisfied none
	// of the first three guards and was re-enqueued every sweep — the same
	// article was dead-lettered 28 times in 6 hours in production. dead_letter
	// jobs must block re-enqueue like an in-flight or completed job does.
	t.Run("skips when a dead_letter job already exists for the article", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-dead").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-dead", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-dead", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "article-dead").Return(true, nil)
		// No HasRecentFailedJob expectation: must not be reached once dead_letter blocks.

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-dead", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.False(t, shouldQueue)
		require.Equal(t, "dead_letter_exists", reason)
	})

	// Guards the failedJobCooldownWindow split from dead_letter: a job that
	// exhausted its retries for a possibly-transient reason (status
	// 'failed', not the explicit ErrContentNotProcessable dead_letter path)
	// must only block re-enqueue for a bounded cooldown, not forever.
	t.Run("skips when the article recently exhausted retries (cooldown)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-recent-fail").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-recent-fail", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-recent-fail", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "article-recent-fail").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), "article-recent-fail", gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, since time.Time) (bool, error) {
				require.WithinDuration(t, time.Now().Add(-1*time.Hour), since, 2*time.Second)
				return true, nil
			},
		)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-recent-fail", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.NoError(t, err)
		require.False(t, shouldQueue)
		require.Equal(t, "recent_failure_cooldown", reason)
	})

	t.Run("propagates the underlying error and stops checking when summaryRepo.Exists fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-err-1").Return(false, assert.AnError)
		// No HasRecentSuccessfulJob/HasInFlightJob expectations set: gomock
		// will fail the test if the guard calls either after Exists errors,
		// which verifies the error path short-circuits the remaining checks.

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-err-1", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.ErrorIs(t, err, assert.AnError)
		require.False(t, shouldQueue)
		require.Empty(t, reason)
	})

	t.Run("propagates the underlying error when HasRecentSuccessfulJob fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-err-2").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-err-2", gomock.Any()).Return(false, assert.AnError)
		// No HasInFlightJob expectation: must not be reached once this errors.

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-err-2", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.ErrorIs(t, err, assert.AnError)
		require.False(t, shouldQueue)
		require.Empty(t, reason)
	})

	t.Run("propagates the underlying error when HasInFlightJob fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-err-3").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-err-3", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-err-3", gomock.Any()).Return(false, assert.AnError)
		// No HasDeadLetterJob expectation: must not be reached once this errors.

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-err-3", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.ErrorIs(t, err, assert.AnError)
		require.False(t, shouldQueue)
		require.Empty(t, reason)
	})

	t.Run("propagates the underlying error when HasDeadLetterJob fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-err-4").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-err-4", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-err-4", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "article-err-4").Return(false, assert.AnError)
		// No HasRecentFailedJob expectation: must not be reached once this errors.

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-err-4", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.ErrorIs(t, err, assert.AnError)
		require.False(t, shouldQueue)
		require.Empty(t, reason)
	})

	t.Run("propagates the underlying error when HasRecentFailedJob fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		mockSummaryRepo.EXPECT().Exists(gomock.Any(), "article-err-5").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), "article-err-5", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasInFlightJob(gomock.Any(), "article-err-5", gomock.Any()).Return(false, nil)
		mockJobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), "article-err-5").Return(false, nil)
		mockJobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), "article-err-5", gomock.Any()).Return(false, assert.AnError)

		shouldQueue, reason, err := service.ShouldQueueSummarizeJob(context.Background(), "article-err-5", mockSummaryRepo, mockJobRepo, testQueueGuardLogger())

		require.ErrorIs(t, err, assert.AnError)
		require.False(t, shouldQueue)
		require.Empty(t, reason)
	})

	// CLAUDE.md rule 8 / ADR-000928: an unwired dependency must fail loudly
	// (panic with a distinguishing message) rather than silently skip the
	// idempotency checks, which would be indistinguishable from a deliberate
	// "disabled" config and would let duplicate summarize jobs/LLM calls
	// through. These guard the panics in ShouldQueueSummarizeJob directly.
	t.Run("panics when summaryRepo is nil (DI wiring forgotten)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockJobRepo := mocks.NewMockSummarizeJobRepository(ctrl)

		assert.PanicsWithValue(t, "ShouldQueueSummarizeJob: summaryRepo is nil (idempotency guard unwired)", func() {
			_, _, _ = service.ShouldQueueSummarizeJob(context.Background(), "article-nil-1", nil, mockJobRepo, testQueueGuardLogger())
		})
	})

	t.Run("panics when jobRepo is nil (DI wiring forgotten)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockSummaryRepo := mocks.NewMockSummaryRepository(ctrl)
		// Exists is allowed to be called before the jobRepo nil check; only
		// stub it if the implementation happens to check jobRepo first.
		mockSummaryRepo.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

		assert.PanicsWithValue(t, "ShouldQueueSummarizeJob: jobRepo is nil (idempotency guard unwired)", func() {
			_, _, _ = service.ShouldQueueSummarizeJob(context.Background(), "article-nil-2", mockSummaryRepo, nil, testQueueGuardLogger())
		})
	})
}
