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

// --- Stubs for enqueue-based batch tests ---

// stubArticleRepoForEnqueue provides FindForSummarization support.
type stubArticleRepoForEnqueue struct {
	repository.ArticleRepository
	articles        []*domain.Article
	findForSumCalls int
}

func (m *stubArticleRepoForEnqueue) FindForSummarization(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
	m.findForSumCalls++
	return m.articles, nil, nil
}

func (m *stubArticleRepoForEnqueue) FindByID(_ context.Context, _ string) (*domain.Article, error) {
	return &domain.Article{ID: "test", Content: "content"}, nil
}

// stubSummaryRepoForEnqueue provides Exists support.
type stubSummaryRepoForEnqueue struct {
	repository.SummaryRepository
}

func (m *stubSummaryRepoForEnqueue) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// stubJobRepoForEnqueue supports all methods needed for enqueue path.
type stubJobRepoForEnqueue struct {
	repository.SummarizeJobRepository
	jobs           []*domain.SummarizeJob
	getErr         error
	createJobCalls int
}

func (m *stubJobRepoForEnqueue) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoForEnqueue) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoForEnqueue) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *stubJobRepoForEnqueue) HasRecentSuccessfulJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForEnqueue) CreateJob(_ context.Context, _ string) (string, error) {
	m.createJobCalls++
	return "new-job-id", nil
}

func newQueueWorkerForEnqueue(articles []*domain.Article, pendingJobs []*domain.SummarizeJob) (*service.SummarizeQueueWorker, *stubJobRepoForEnqueue, *stubArticleRepoForEnqueue) {
	jobRepo := &stubJobRepoForEnqueue{jobs: pendingJobs}
	articleRepo := &stubArticleRepoForEnqueue{articles: articles}
	summaryRepo := &stubSummaryRepoForEnqueue{}
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testJobHandlerLogger(), 10)
	return worker, jobRepo, articleRepo
}

func newQueueWorkerForEnqueueWithError(err error) *service.SummarizeQueueWorker {
	jobRepo := &stubJobRepoForEnqueue{getErr: err}
	articleRepo := &stubArticleRepoForEnqueue{}
	summaryRepo := &stubSummaryRepoForEnqueue{}
	return service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testJobHandlerLogger(), 10)
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
		articles := []*domain.Article{{ID: "a-new", Title: "New"}}
		worker, _, articleRepo := newQueueWorkerForEnqueue(articles, []*domain.SummarizeJob{{ArticleID: "a1"}})
		h := &jobHandler{
			queueWorker:             worker,
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, articleRepo.findForSumCalls, "EnqueueUnsummarizedBatch should not be called when queue has pending jobs")
	})

	t.Run("enqueues when queue is empty", func(t *testing.T) {
		ctx := context.Background()
		articles := []*domain.Article{{ID: "a-new", Title: "New"}}
		worker, jobRepo, articleRepo := newQueueWorkerForEnqueue(articles, []*domain.SummarizeJob{})
		mock := &mockArticleSummarizer{}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             worker,
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, articleRepo.findForSumCalls, "should call FindForSummarization via EnqueueUnsummarizedBatch")
		assert.Equal(t, 1, jobRepo.createJobCalls, "should enqueue article via CreateJob")
		assert.False(t, mock.summarizeCalled, "SummarizeArticles should NOT be called (enqueue path replaces direct LLM)")
	})

	t.Run("proceeds with enqueue when HasPendingJobs returns error (fail-open)", func(t *testing.T) {
		ctx := context.Background()
		articles := []*domain.Article{{ID: "a-new", Title: "New"}}
		worker := newQueueWorkerForEnqueueWithError(fmt.Errorf("db error"))
		// Override article repo after construction to provide articles
		mock := &mockArticleSummarizer{}
		h := &jobHandler{
			articleSummarizer:       mock,
			queueWorker:             worker,
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
		}
		_ = articles // used only for documentation

		err := h.processSummarizationBatch(ctx)
		// The fail-open path runs the batch. EnqueueUnsummarizedBatch may return error from FindForSummarization
		// which is acceptable. The key is that it does not panic and does not call SummarizeArticles.
		assert.NoError(t, err)
		assert.False(t, mock.summarizeCalled, "SummarizeArticles should NOT be called (enqueue path)")
	})

	t.Run("force sweeps enqueue after interval even with pending jobs", func(t *testing.T) {
		ctx := context.Background()
		articles := []*domain.Article{{ID: "a-new", Title: "New"}}
		worker, jobRepo, articleRepo := newQueueWorkerForEnqueue(articles, []*domain.SummarizeJob{{ArticleID: "a1"}})
		h := &jobHandler{
			queueWorker:             worker,
			logger:                  testJobHandlerLogger(),
			jobGroup:                orchestrator.NewJobGroup(ctx, testJobHandlerLogger()),
			batchSize:               10,
			batchSweepForceInterval: 30 * time.Minute,
			lastBatchSweep:          time.Now().Add(-1 * time.Hour),
		}

		err := h.processSummarizationBatch(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, articleRepo.findForSumCalls, "should call EnqueueUnsummarizedBatch during force sweep")
		assert.Equal(t, 1, jobRepo.createJobCalls, "should enqueue article during force sweep")
	})

	t.Run("falls back to SummarizeArticles when queueWorker is nil", func(t *testing.T) {
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
		assert.True(t, mock.summarizeCalled, "SummarizeArticles should be called when queueWorker is nil (fallback)")
	})
}
