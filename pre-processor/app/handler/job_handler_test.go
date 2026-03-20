package handler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/orchestrator"
	"pre-processor/repository"
	"pre-processor/service"

	"github.com/stretchr/testify/assert"
)

func testJobHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// mockArticleSummarizer tracks calls for testing cursor reset behavior.
type mockArticleSummarizer struct {
	result             *service.SummarizationResult
	err                error
	resetCalled        bool
	summarizeCalled    bool
	hasUnsummarized    bool
	hasUnsummarizedErr error
}

func (m *mockArticleSummarizer) SummarizeArticles(_ context.Context, _ int) (*service.SummarizationResult, error) {
	m.summarizeCalled = true
	return m.result, m.err
}

func (m *mockArticleSummarizer) HasUnsummarizedArticles(_ context.Context) (bool, error) {
	return m.hasUnsummarized, m.hasUnsummarizedErr
}

func (m *mockArticleSummarizer) ResetPagination() error {
	m.resetCalled = true
	return nil
}

// mockQualityChecker tracks calls for testing cursor reset behavior.
type mockQualityChecker struct {
	result      *service.QualityResult
	err         error
	resetCalled bool
	checkCalled bool
}

func (m *mockQualityChecker) CheckQuality(_ context.Context, _ int) (*service.QualityResult, error) {
	m.checkCalled = true
	return m.result, m.err
}

func (m *mockQualityChecker) ProcessLowQualityArticles(_ context.Context, _ []domain.ArticleWithSummary) error {
	return nil
}

func (m *mockQualityChecker) ResetPagination() error {
	m.resetCalled = true
	return nil
}

// panickingArticleSummarizer is a mock that panics when SummarizeArticles is called.
type panickingArticleSummarizer struct{}

func (p *panickingArticleSummarizer) SummarizeArticles(_ context.Context, _ int) (*service.SummarizationResult, error) {
	panic("simulated panic in summarization")
}

func (p *panickingArticleSummarizer) HasUnsummarizedArticles(_ context.Context) (bool, error) {
	return false, nil
}

func (p *panickingArticleSummarizer) ResetPagination() error {
	return nil
}

func TestProcessSummarizationBatch_PanicOnSummarizerPanic(t *testing.T) {
	t.Run("processSummarizationBatch panics when summarizer panics", func(t *testing.T) {
		ctx := context.Background()

		h := &jobHandler{
			articleSummarizer: &panickingArticleSummarizer{},
			logger:            testJobHandlerLogger(),
			jobGroup:          orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:         10,
		}

		// processSummarizationBatch should propagate the panic (no recover there).
		// The orchestrator's JobRunner has the recover() that catches it.
		assert.Panics(t, func() {
			_ = h.processSummarizationBatch(ctx)
		}, "processSummarizationBatch should panic when summarizer panics")
	})
}

func TestProcessSummarizationBatch_CursorReset(t *testing.T) {
	t.Run("resets pagination when HasMore=false and ProcessedCount>0", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 5,
				SuccessCount:   5,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			articleSummarizer: mock,
			logger:            testJobHandlerLogger(),
			jobGroup:          orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:         10,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.resetCalled, "should reset pagination when scan ends with processed items")
	})

	t.Run("resets pagination when HasMore=false and ProcessedCount=0", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 0,
				SuccessCount:   0,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			articleSummarizer: mock,
			logger:            testJobHandlerLogger(),
			jobGroup:          orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:         10,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.resetCalled, "should reset pagination when scan ends even with zero processed")
	})

	t.Run("does not reset pagination when HasMore=true", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 10,
				SuccessCount:   10,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			articleSummarizer: mock,
			logger:            testJobHandlerLogger(),
			jobGroup:          orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:         10,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.False(t, mock.resetCalled, "should not reset pagination when there are more articles")
	})
}

func TestProcessQualityCheckBatch_CursorReset(t *testing.T) {
	t.Run("resets pagination when HasMore=false and ProcessedCount=0", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 0,
				SuccessCount:   0,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			qualityChecker: mock,
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.resetCalled, "should reset pagination when scan ends even with zero processed")
	})

	t.Run("does not reset pagination when HasMore=true", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 10,
				SuccessCount:   10,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			qualityChecker: mock,
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
		assert.False(t, mock.resetCalled, "should not reset pagination when there are more articles")
	})
}

// stubJobRepoForHandler is a minimal stub for SummarizeJobRepository used to construct SummarizeQueueWorker in handler tests.
type stubJobRepoForHandler struct {
	repository.SummarizeJobRepository
	jobs   []*domain.SummarizeJob
	getErr error
}

func (m *stubJobRepoForHandler) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoForHandler) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoForHandler) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

func newQueueWorkerWithJobs(jobs []*domain.SummarizeJob) *service.SummarizeQueueWorker {
	return service.NewSummarizeQueueWorker(
		&stubJobRepoForHandler{jobs: jobs},
		nil, nil, nil,
		testJobHandlerLogger(),
		10,
	)
}

func newQueueWorkerWithError(err error) *service.SummarizeQueueWorker {
	return service.NewSummarizeQueueWorker(
		&stubJobRepoForHandler{getErr: err},
		nil, nil, nil,
		testJobHandlerLogger(),
		10,
	)
}

func TestProcessQualityCheckBatch_SkipsWhenSummarizationPending(t *testing.T) {
	t.Run("skips quality check when summarization queue has pending jobs", func(t *testing.T) {
		ctx := context.Background()
		qcMock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 5,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			qualityChecker: qcMock,
			queueWorker:    newQueueWorkerWithJobs([]*domain.SummarizeJob{{ArticleID: "a1"}}),
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
		assert.False(t, qcMock.checkCalled, "CheckQuality should not have been called when summarization has pending jobs")
	})

	t.Run("runs quality check when summarization queue is empty", func(t *testing.T) {
		ctx := context.Background()
		qcMock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 5,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			qualityChecker: qcMock,
			queueWorker:    newQueueWorkerWithJobs([]*domain.SummarizeJob{}),
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
		assert.False(t, qcMock.resetCalled, "should not reset when HasMore=true")
	})

	t.Run("proceeds with quality check when HasPendingJobs returns error", func(t *testing.T) {
		ctx := context.Background()
		qcMock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 3,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			qualityChecker: qcMock,
			queueWorker:    newQueueWorkerWithError(fmt.Errorf("db error")),
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, qcMock.resetCalled, "should reset pagination since HasMore=false")
	})

	t.Run("runs quality check when queueWorker is nil", func(t *testing.T) {
		ctx := context.Background()
		qcMock := &mockQualityChecker{
			result: &service.QualityResult{
				ProcessedCount: 2,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			qualityChecker: qcMock,
			queueWorker:    nil,
			logger:         testJobHandlerLogger(),
			jobGroup:       orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:      10,
		}

		err := h.processQualityCheckBatch(ctx)
		assert.NoError(t, err)
	})
}

func TestProcessSummarizationBatch_DefersWhenQueueHasPendingJobs(t *testing.T) {
	t.Run("defers batch when queue has pending jobs", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 5,
				SuccessCount:   5,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             newQueueWorkerWithJobs([]*domain.SummarizeJob{{ArticleID: "a1"}}),
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.False(t, mock.summarizeCalled, "SummarizeArticles should not be called when queue has pending jobs")
	})

	t.Run("runs batch when queue is empty", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 5,
				SuccessCount:   5,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             newQueueWorkerWithJobs([]*domain.SummarizeJob{}),
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.summarizeCalled, "SummarizeArticles should be called when queue is empty")
	})

	t.Run("proceeds with batch when HasPendingJobs returns error (fail-open)", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 3,
				SuccessCount:   3,
				HasMore:        false,
			},
		}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             newQueueWorkerWithError(fmt.Errorf("db error")),
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.summarizeCalled, "SummarizeArticles should be called on HasPendingJobs error (fail-open)")
	})

	t.Run("force sweeps after interval even with pending jobs", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 2,
				SuccessCount:   2,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             newQueueWorkerWithJobs([]*domain.SummarizeJob{{ArticleID: "a1"}}),
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
			lastBatchSweep:          time.Now().Add(-1 * time.Hour),
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.summarizeCalled, "SummarizeArticles should be called for force sweep after interval")
	})

	t.Run("runs batch normally when queueWorker is nil", func(t *testing.T) {
		ctx := context.Background()
		mock := &mockArticleSummarizer{
			result: &service.SummarizationResult{
				ProcessedCount: 5,
				SuccessCount:   5,
				HasMore:        true,
			},
		}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             nil,
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.True(t, mock.summarizeCalled, "SummarizeArticles should be called when queueWorker is nil")
	})
}
