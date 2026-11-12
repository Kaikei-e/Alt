package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/orchestrator"
	"pre-processor/repository"
	"pre-processor/service"
	"pre-processor/test/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	)
}

func newQueueWorkerWithError(err error) *service.SummarizeQueueWorker {
	return service.NewSummarizeQueueWorker(
		&stubJobRepoForHandler{getErr: err},
		nil, nil, nil,
		testJobHandlerLogger(),
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

func (m *stubJobRepoForEnqueue) HasInFlightJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForEnqueue) HasDeadLetterJob(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForEnqueue) HasRecentFailedJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForEnqueue) CountInvalidatedCompletedJobs(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *stubJobRepoForEnqueue) CreateJob(_ context.Context, _ string) (string, error) {
	m.createJobCalls++
	return "new-job-id", nil
}

func newQueueWorkerForEnqueue(articles []*domain.Article, pendingJobs []*domain.SummarizeJob) (*service.SummarizeQueueWorker, *stubJobRepoForEnqueue, *stubArticleRepoForEnqueue) {
	jobRepo := &stubJobRepoForEnqueue{jobs: pendingJobs}
	articleRepo := &stubArticleRepoForEnqueue{articles: articles}
	summaryRepo := &stubSummaryRepoForEnqueue{}
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testJobHandlerLogger())
	return worker, jobRepo, articleRepo
}

func newQueueWorkerForEnqueueWithError(err error) *service.SummarizeQueueWorker {
	jobRepo := &stubJobRepoForEnqueue{getErr: err}
	articleRepo := &stubArticleRepoForEnqueue{}
	summaryRepo := &stubSummaryRepoForEnqueue{}
	return service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testJobHandlerLogger())
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

type scriptedFindCall struct {
	articles []*domain.Article
	cursor   *domain.Cursor
	err      error
}

type stubArticleRepoForRefill struct {
	repository.ArticleRepository
	calls []scriptedFindCall
	idx   int
}

func (m *stubArticleRepoForRefill) FindForSummarization(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
	i := m.idx
	m.idx++
	if i >= len(m.calls) {
		return nil, nil, nil
	}
	c := m.calls[i]
	return c.articles, c.cursor, c.err
}

func (m *stubArticleRepoForRefill) FindByID(_ context.Context, id string) (*domain.Article, error) {
	return &domain.Article{ID: id, Content: "content"}, nil
}

type stubJobRepoForRefill struct {
	repository.SummarizeJobRepository
	pendingJobs    []*domain.SummarizeJob
	getPendingErr  error
	dequeueJobs    []*domain.SummarizeJob
	createJobToken string
	createJobCalls int
}

func (m *stubJobRepoForRefill) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getPendingErr != nil {
		return nil, m.getPendingErr
	}
	return m.pendingJobs, nil
}

func (m *stubJobRepoForRefill) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	jobs := m.dequeueJobs
	m.dequeueJobs = nil
	return jobs, nil
}

func (m *stubJobRepoForRefill) UpdateJobStatus(_ context.Context, _ string, _ domain.SummarizeJobStatus, _, _ string) error {
	return nil
}

func (m *stubJobRepoForRefill) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *stubJobRepoForRefill) HasRecentSuccessfulJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForRefill) HasInFlightJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForRefill) HasDeadLetterJob(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForRefill) HasRecentFailedJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoForRefill) CountInvalidatedCompletedJobs(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *stubJobRepoForRefill) CreateJob(_ context.Context, _ string) (string, error) {
	m.createJobCalls++
	return m.createJobToken, nil
}

type stubExternalAPIRepoForRefill struct {
	repository.ExternalAPIRepository
	summarizeErr error
}

func (m *stubExternalAPIRepoForRefill) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	return nil, m.summarizeErr
}

func makeArticles(n int) []*domain.Article {
	out := make([]*domain.Article, n)
	for i := range n {
		out[i] = &domain.Article{ID: fmt.Sprintf("article-%d", i+1), Title: "Title"}
	}
	return out
}

func TestProcessSummarizationBatch_RefillStateMachine(t *testing.T) {
	tests := []struct {
		name              string
		findCalls         []scriptedFindCall
		createJobToken    string
		pendingOnTick     []*domain.SummarizeJob
		wantFirstSweepErr bool
		wantTotalSweeps   int
		wantRefillLogged  bool
	}{
		{
			name: "{HasMore true, Enqueued 10, nil} then empty queue tick runs refill sweep",
			findCalls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
				{articles: makeArticles(3), cursor: nil},
			},
			createJobToken:    "job-id",
			pendingOnTick:     nil,
			wantFirstSweepErr: false,
			wantTotalSweeps:   2,
			wantRefillLogged:  true,
		},
		{
			name: "{HasMore true, Enqueued 0} does not refill",
			findCalls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
				{articles: makeArticles(3), cursor: nil},
			},
			createJobToken:    "",
			pendingOnTick:     nil,
			wantFirstSweepErr: false,
			wantTotalSweeps:   1,
			wantRefillLogged:  false,
		},
		{
			name: "sweep error does not refill",
			findCalls: []scriptedFindCall{
				{articles: nil, cursor: nil, err: errors.New("backend failure")},
				{articles: makeArticles(3), cursor: nil},
			},
			createJobToken:    "job-id",
			pendingOnTick:     nil,
			wantFirstSweepErr: true,
			wantTotalSweeps:   1,
			wantRefillLogged:  false,
		},
		{
			name: "{HasMore false} does not refill",
			findCalls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: nil},
				{articles: makeArticles(3), cursor: nil},
			},
			createJobToken:    "job-id",
			pendingOnTick:     nil,
			wantFirstSweepErr: false,
			wantTotalSweeps:   1,
			wantRefillLogged:  false,
		},
		{
			name: "refill mode but pending jobs present does not sweep",
			findCalls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
				{articles: makeArticles(3), cursor: nil},
			},
			createJobToken:    "job-id",
			pendingOnTick:     []*domain.SummarizeJob{{ArticleID: "pending-1"}},
			wantFirstSweepErr: false,
			wantTotalSweeps:   1,
			wantRefillLogged:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var logBuf bytes.Buffer
			log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

			jobRepo := &stubJobRepoForRefill{createJobToken: tc.createJobToken}
			articleRepo := &stubArticleRepoForRefill{calls: tc.findCalls}
			summaryRepo := &stubSummaryRepoForEnqueue{}
			worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, log)

			h := &jobHandler{
				queueWorker:             worker,
				logger:                  log,
				jobGroup:                orchestrator.NewJobGroup(ctx, log),
				batchSize:               10,
				lastBatchSweep:          time.Now(),
				batchSweepForceInterval: 30 * time.Minute,
			}

			err := h.processSummarizationBatch(ctx)
			if tc.wantFirstSweepErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			jobRepo.pendingJobs = tc.pendingOnTick
			require.NoError(t, h.processQueueTick(ctx))

			assert.Equal(t, tc.wantTotalSweeps, articleRepo.idx, "unexpected number of FindForSummarization sweeps")
			if tc.wantRefillLogged {
				assert.Contains(t, logBuf.String(), "summarization refill sweep")
				assert.Contains(t, logBuf.String(), "enqueued=3")
				assert.Contains(t, logBuf.String(), "has_more=false")
			} else {
				assert.NotContains(t, logBuf.String(), "summarization refill sweep")
			}
		})
	}

	t.Run("ProcessQueue returns ErrServiceOverloaded skips refill sweep and keeps refill flag", func(t *testing.T) {
		ctx := context.Background()
		var logBuf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		jobRepo := &stubJobRepoForRefill{createJobToken: "job-id"}
		articleRepo := &stubArticleRepoForRefill{
			calls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
				{articles: makeArticles(3), cursor: nil},
			},
		}
		apiRepo := &stubExternalAPIRepoForRefill{summarizeErr: domain.ErrServiceOverloaded}
		summaryRepo := &stubSummaryRepoForEnqueue{}
		worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log)

		h := &jobHandler{
			queueWorker:             worker,
			logger:                  log,
			jobGroup:                orchestrator.NewJobGroup(ctx, log),
			batchSize:               10,
			lastBatchSweep:          time.Now(),
			batchSweepForceInterval: 30 * time.Minute,
		}

		require.NoError(t, h.processSummarizationBatch(ctx))
		require.True(t, h.refillNeeded)

		jobRepo.dequeueJobs = []*domain.SummarizeJob{{ArticleID: "article-1", MaxRetries: 3}}
		err := h.processQueueTick(ctx)
		require.ErrorIs(t, err, domain.ErrServiceOverloaded)
		assert.Equal(t, 1, articleRepo.idx, "refill sweep must not run when ProcessQueue fails")
		assert.True(t, h.refillNeeded, "refill flag must be kept when ProcessQueue returns ErrServiceOverloaded")
		assert.NotContains(t, logBuf.String(), "summarization refill sweep")
	})

	t.Run("HasPendingJobs error defers refill sweep and keeps flag for next tick", func(t *testing.T) {
		ctx := context.Background()
		var logBuf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		jobRepo := &stubJobRepoForRefill{createJobToken: "job-id"}
		articleRepo := &stubArticleRepoForRefill{
			calls: []scriptedFindCall{
				{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
				{articles: makeArticles(3), cursor: nil},
			},
		}
		summaryRepo := &stubSummaryRepoForEnqueue{}
		worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, log)

		h := &jobHandler{
			queueWorker:             worker,
			logger:                  log,
			jobGroup:                orchestrator.NewJobGroup(ctx, log),
			batchSize:               10,
			lastBatchSweep:          time.Now(),
			batchSweepForceInterval: 30 * time.Minute,
		}

		require.NoError(t, h.processSummarizationBatch(ctx))
		require.True(t, h.refillNeeded)

		jobRepo.getPendingErr = errors.New("db transient error")
		require.NoError(t, h.processQueueTick(ctx))
		assert.Equal(t, 1, articleRepo.idx, "refill sweep must not run when HasPendingJobs fails")
		assert.True(t, h.refillNeeded, "refill flag must be kept when HasPendingJobs fails")
		assert.NotContains(t, logBuf.String(), "summarization refill sweep")

		jobRepo.getPendingErr = nil
		jobRepo.pendingJobs = nil
		require.NoError(t, h.processQueueTick(ctx))
		assert.Equal(t, 2, articleRepo.idx, "next tick with no pending jobs must run refill sweep")
		assert.False(t, h.refillNeeded, "refill flag cleared when refill sweep reaches end")
		assert.Contains(t, logBuf.String(), "summarization refill sweep")
	})
}

func TestProcessQueueTick_RefillsAndProcessesWhileStragglerInFlight(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	var queueMu sync.Mutex
	var pending []*domain.SummarizeJob

	enqueueJob := func(articleID string) string {
		queueMu.Lock()
		defer queueMu.Unlock()
		j := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: articleID, MaxRetries: 3}
		pending = append(pending, j)
		return j.JobID.String()
	}

	var sweepCall atomic.Int32
	articleRepo.EXPECT().FindForSummarization(gomock.Any(), gomock.Any(), 2).DoAndReturn(
		func(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
			switch sweepCall.Add(1) {
			case 1:
				return []*domain.Article{
					{ID: "straggler-1", Title: "S1"},
					{ID: "fast-2", Title: "F2"},
				}, &domain.Cursor{LastID: "fast-2"}, nil
			case 2:
				return []*domain.Article{
					{ID: "refill-3", Title: "R3"},
				}, nil, nil
			default:
				t.Errorf("unexpected extra FindForSummarization sweep")
				return nil, nil, nil
			}
		}).Times(2)

	summaryRepo.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasInFlightJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()

	jobRepo.EXPECT().CreateJob(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, articleID string) (string, error) {
		return enqueueJob(articleID), nil
	}).Times(3)

	jobRepo.EXPECT().GetPendingJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		queueMu.Lock()
		defer queueMu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		return []*domain.SummarizeJob{pending[0]}, nil
	}).AnyTimes()

	stragglerStarted := make(chan struct{})
	releaseStraggler := make(chan struct{})
	refill3Completed := make(chan struct{})
	var closeRefill3Once sync.Once

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, limit int) ([]*domain.SummarizeJob, error) {
		if limit != 1 {
			t.Errorf("expected on-demand DequeueJobs(ctx, 1), got limit=%d", limit)
			closeRefill3Once.Do(func() { close(refill3Completed) })
			return nil, fmt.Errorf("unexpected batch dequeue limit %d", limit)
		}
		queueMu.Lock()
		defer queueMu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		j := pending[0]
		pending = pending[1:]
		return []*domain.SummarizeJob{j}, nil
	}).AnyTimes()

	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "Title", Content: "Body"}, nil
	}).AnyTimes()

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, a *domain.Article, _ string) (*domain.SummarizedContent, error) {
		switch a.ID {
		case "straggler-1":
			close(stragglerStarted)
			select {
			case <-releaseStraggler:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting to release straggler-1")
			}
		case "fast-2":
			select {
			case <-stragglerStarted:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for straggler-1 to start")
			}
		}
		return &domain.SummarizedContent{SummaryJapanese: "要約:" + a.ID}, nil
	}).AnyTimes()

	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _, articleID string) error {
			if articleID == "refill-3" {
				closeRefill3Once.Do(func() { close(refill3Completed) })
			}
			return nil
		}).AnyTimes()

	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log)
	worker.SetConcurrency(2)

	h := &jobHandler{
		queueWorker:             worker,
		logger:                  log,
		jobGroup:                orchestrator.NewJobGroup(ctx, log),
		batchSize:               2,
		lastBatchSweep:          time.Now(),
		batchSweepForceInterval: 30 * time.Minute,
	}

	require.NoError(t, h.processSummarizationBatch(ctx))
	require.True(t, h.refillNeeded)

	tickDone := make(chan error, 1)
	go func() {
		tickDone <- h.processQueueTick(ctx)
	}()

	// refill-3 must be enqueued and completed by the free worker while straggler-1 is still blocked.
	select {
	case <-refill3Completed:
	case <-time.After(5 * time.Second):
		close(releaseStraggler)
		<-tickDone
		t.Fatal("timed out waiting for refill-3 to complete while straggler-1 was in flight")
	}
	select {
	case err := <-tickDone:
		t.Fatalf("processQueueTick returned before straggler finished: %v", err)
	default:
	}

	close(releaseStraggler)
	select {
	case err := <-tickDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processQueueTick to finish")
	}
	assert.Contains(t, logBuf.String(), "summarization refill sweep")
	assert.False(t, h.refillNeeded)
}

func TestProcessQueueTick_ConcurrentDrainedWorkersDoNotTriggerDuplicateSweeps(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)

	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	var queueMu sync.Mutex
	var pending []*domain.SummarizeJob

	var sweepCall atomic.Int32
	articleRepo.EXPECT().FindForSummarization(gomock.Any(), gomock.Any(), 3).DoAndReturn(
		func(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
			switch sweepCall.Add(1) {
			case 1:
				return []*domain.Article{
					{ID: "init-1", Title: "I1"},
				}, &domain.Cursor{LastID: "init-1"}, nil
			case 2:
				return []*domain.Article{
					{ID: "refill-1", Title: "R1"},
				}, &domain.Cursor{LastID: "refill-1"}, nil
			case 3:
				// Enqueues 0 despite HasMore=true -> must terminate refill loop.
				return []*domain.Article{}, &domain.Cursor{LastID: "refill-1"}, nil
			default:
				t.Errorf("duplicate refill sweep triggered")
				return nil, nil, nil
			}
		}).Times(3)

	summaryRepo.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasInFlightJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()

	jobRepo.EXPECT().CreateJob(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, articleID string) (string, error) {
		queueMu.Lock()
		defer queueMu.Unlock()
		j := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: articleID, MaxRetries: 3}
		pending = append(pending, j)
		return j.JobID.String(), nil
	}).Times(2)

	jobRepo.EXPECT().GetPendingJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		queueMu.Lock()
		defer queueMu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		return []*domain.SummarizeJob{pending[0]}, nil
	}).AnyTimes()

	allThreeDrained := make(chan struct{})
	var closeDrainedOnce sync.Once
	var firstDrainCount atomic.Int32

	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, limit int) ([]*domain.SummarizeJob, error) {
		if limit != 1 {
			t.Errorf("expected on-demand DequeueJobs(ctx, 1), got limit=%d", limit)
			closeDrainedOnce.Do(func() { close(allThreeDrained) })
			return nil, fmt.Errorf("unexpected batch dequeue limit %d", limit)
		}
		queueMu.Lock()
		if len(pending) > 0 {
			j := pending[0]
			pending = pending[1:]
			queueMu.Unlock()
			return []*domain.SummarizeJob{j}, nil
		}
		queueMu.Unlock()

		if firstDrainCount.Add(1) <= 3 {
			if firstDrainCount.Load() == 3 {
				closeDrainedOnce.Do(func() { close(allThreeDrained) })
			}
			select {
			case <-allThreeDrained:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for all three workers to drain")
			}
		}
		return nil, nil
	}).AnyTimes()

	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "Title", Content: "Body"}, nil
	}).AnyTimes()
	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").Return(&domain.SummarizedContent{SummaryJapanese: "要約"}, nil).AnyTimes()
	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).Return(nil).AnyTimes()

	log := testJobHandlerLogger()
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log)
	worker.SetConcurrency(3)

	h := &jobHandler{
		queueWorker:             worker,
		logger:                  log,
		jobGroup:                orchestrator.NewJobGroup(ctx, log),
		batchSize:               3,
		lastBatchSweep:          time.Now(),
		batchSweepForceInterval: 30 * time.Minute,
	}

	require.NoError(t, h.processSummarizationBatch(ctx))
	// Clear initial job so all 3 workers immediately drain concurrently on tick start.
	queueMu.Lock()
	pending = nil
	queueMu.Unlock()

	require.NoError(t, h.processQueueTick(ctx))
	assert.Equal(t, int32(3), sweepCall.Load(), "must run initial sweep + 1 refill sweep + 1 terminating empty sweep without duplicates")
	assert.False(t, h.refillNeeded)
}

func TestProcessQueueTick_PropagatesRefillError(t *testing.T) {
	ctx := context.Background()
	refillErr := errors.New("refill sweep failed")

	jobRepo := &stubJobRepoForRefill{createJobToken: "job-id"}
	articleRepo := &stubArticleRepoForRefill{
		calls: []scriptedFindCall{
			{articles: makeArticles(10), cursor: &domain.Cursor{LastID: "article-10"}},
			{articles: nil, cursor: nil, err: refillErr},
		},
	}
	summaryRepo := &stubSummaryRepoForEnqueue{}
	log := testJobHandlerLogger()
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, log)

	h := &jobHandler{
		queueWorker:             worker,
		logger:                  log,
		jobGroup:                orchestrator.NewJobGroup(ctx, log),
		batchSize:               10,
		lastBatchSweep:          time.Now(),
		batchSweepForceInterval: 30 * time.Minute,
	}

	require.NoError(t, h.processSummarizationBatch(ctx))
	require.True(t, h.refillNeeded)

	err := h.processQueueTick(ctx)
	require.ErrorIs(t, err, refillErr)
	assert.False(t, h.refillNeeded)
}

func TestProcessQueueTick_WorkerDoesNotExitWhileRefillStillNeeded(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	var mu sync.Mutex
	var pending []*domain.SummarizeJob
	var emptyDequeues atomic.Int32
	secondEmpty := make(chan struct{})
	var secondEmptyOnce sync.Once
	r1Dequeued := make(chan struct{})
	var r1DequeuedOnce sync.Once
	var afterSweep2 atomic.Bool

	var sweepCall atomic.Int32
	articleRepo.EXPECT().FindForSummarization(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
			switch sweepCall.Add(1) {
			case 1:
				return []*domain.Article{{ID: "straggler"}, {ID: "fast-2"}}, &domain.Cursor{LastID: "x"}, nil
			case 2:
				select {
				case <-secondEmpty:
				case <-time.After(5 * time.Second):
					t.Error("timed out waiting for second empty dequeue")
				}
				time.Sleep(100 * time.Millisecond) // let the second drained worker block on sweepMu
				return []*domain.Article{{ID: "r1"}}, &domain.Cursor{LastID: "y"}, nil
			case 3:
				return []*domain.Article{{ID: "r2"}, {ID: "r3"}}, nil, nil
			default:
				return nil, nil, nil
			}
		}).AnyTimes()
	summaryRepo.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasInFlightJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
	jobRepo.EXPECT().CreateJob(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		j := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: id, MaxRetries: 3}
		pending = append(pending, j)
		if id == "r1" {
			afterSweep2.Store(true)
		}
		return j.JobID.String(), nil
	}).AnyTimes()
	jobRepo.EXPECT().GetPendingJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		if afterSweep2.Load() {
			select {
			case <-r1Dequeued:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for r1 to be dequeued")
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		return []*domain.SummarizeJob{pending[0]}, nil
	}).AnyTimes()
	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil).AnyTimes()
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(pending) == 0 {
			if emptyDequeues.Add(1) == 2 {
				secondEmptyOnce.Do(func() { close(secondEmpty) })
			}
			return nil, nil
		}
		j := pending[0]
		pending = pending[1:]
		if j.ArticleID == "r1" {
			r1DequeuedOnce.Do(func() { close(r1Dequeued) })
		}
		return []*domain.SummarizeJob{j}, nil
	}).AnyTimes()
	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "T", Content: "Body"}, nil
	}).AnyTimes()

	stragglerStarted := make(chan struct{})
	releaseStraggler := make(chan struct{})
	var releaseStragglerOnce sync.Once
	closeReleaseStraggler := func() {
		releaseStragglerOnce.Do(func() { close(releaseStraggler) })
	}
	r3Started := make(chan struct{})
	var r3StartedOnce sync.Once
	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, a *domain.Article, _ string) (*domain.SummarizedContent, error) {
		switch a.ID {
		case "straggler":
			close(stragglerStarted)
			select {
			case <-releaseStraggler:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting to release straggler")
			}
		case "fast-2":
			select {
			case <-stragglerStarted:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for straggler to start")
			}
		case "r1":
			time.Sleep(200 * time.Millisecond)
		case "r2":
			select {
			case <-r3Started:
			case <-releaseStraggler:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for r3 to start while r2 was in flight")
			}
		case "r3":
			r3StartedOnce.Do(func() { close(r3Started) })
		}
		return &domain.SummarizedContent{SummaryJapanese: "s"}, nil
	}).AnyTimes()
	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	r3Done := make(chan struct{})
	var r3DoneOnce sync.Once
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).DoAndReturn(func(_ context.Context, _, _, _, id string) error {
		if id == "r3" {
			r3DoneOnce.Do(func() { close(r3Done) })
		}
		return nil
	}).AnyTimes()

	log := testJobHandlerLogger()
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log)
	worker.SetConcurrency(3)
	h := &jobHandler{queueWorker: worker, logger: log, jobGroup: orchestrator.NewJobGroup(ctx, log), batchSize: 3, lastBatchSweep: time.Now(), batchSweepForceInterval: 30 * time.Minute}
	require.NoError(t, h.processSummarizationBatch(ctx))

	done := make(chan error, 1)
	go func() { done <- h.processQueueTick(ctx) }()

	select {
	case <-r3Started:
	case <-time.After(1500 * time.Millisecond):
		closeReleaseStraggler()
		<-done
		t.Fatal("r3 did not start while straggler and r2 were in flight: a drained worker exited although refillNeeded was still true")
	}

	select {
	case <-r3Done:
	case <-time.After(5 * time.Second):
		closeReleaseStraggler()
		<-done
		t.Fatal("timed out waiting for r3 to complete")
	}

	closeReleaseStraggler()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processQueueTick to finish")
	}
}

func TestProcessQueueTick_PendingJobMissedByDequeuePickedUpWhileStragglerInFlight(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	jobRepo := mocks.NewMockSummarizeJobRepository(ctrl)
	articleRepo := mocks.NewMockArticleRepository(ctrl)
	apiRepo := mocks.NewMockExternalAPIRepository(ctrl)
	summaryRepo := mocks.NewMockSummaryRepository(ctrl)

	var mu sync.Mutex
	var pending []*domain.SummarizeJob

	var sweepCall atomic.Int32
	articleRepo.EXPECT().FindForSummarization(gomock.Any(), gomock.Any(), 2).DoAndReturn(
		func(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
			switch sweepCall.Add(1) {
			case 1:
				return []*domain.Article{{ID: "straggler-1"}, {ID: "missed-2"}}, &domain.Cursor{LastID: "missed-2"}, nil
			case 2:
				return nil, nil, nil
			default:
				t.Errorf("unexpected extra FindForSummarization sweep")
				return nil, nil, nil
			}
		}).AnyTimes()
	summaryRepo.EXPECT().Exists(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentSuccessfulJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasInFlightJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasDeadLetterJob(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().HasRecentFailedJob(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	jobRepo.EXPECT().CountInvalidatedCompletedJobs(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
	jobRepo.EXPECT().CreateJob(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		j := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: id, MaxRetries: 3}
		pending = append(pending, j)
		return j.JobID.String(), nil
	}).Times(2)

	jobRepo.EXPECT().GetPendingJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(pending) == 0 {
			return nil, nil
		}
		return []*domain.SummarizeJob{pending[0]}, nil
	}).AnyTimes()
	jobRepo.EXPECT().RecoverStuckJobs(gomock.Any()).Return(int64(0), nil)

	var dequeueCalls atomic.Int32
	jobRepo.EXPECT().DequeueJobs(gomock.Any(), 1).DoAndReturn(func(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
		call := dequeueCalls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		// Call 2 simulates a racing dequeue / SKIP LOCKED that misses the pending row.
		if call == 2 || len(pending) == 0 {
			return nil, nil
		}
		j := pending[0]
		pending = pending[1:]
		return []*domain.SummarizeJob{j}, nil
	}).AnyTimes()

	articleRepo.EXPECT().FindByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*domain.Article, error) {
		return &domain.Article{ID: id, UserID: "u1", Title: "T", Content: "Body"}, nil
	}).AnyTimes()

	releaseStraggler := make(chan struct{})
	var releaseOnce sync.Once
	closeRelease := func() { releaseOnce.Do(func() { close(releaseStraggler) }) }
	missed2Done := make(chan struct{})
	var missed2DoneOnce sync.Once

	apiRepo.EXPECT().SummarizeArticle(gomock.Any(), gomock.Any(), "low").DoAndReturn(func(_ context.Context, a *domain.Article, _ string) (*domain.SummarizedContent, error) {
		if a.ID == "straggler-1" {
			select {
			case <-releaseStraggler:
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting to release straggler-1")
			}
		}
		return &domain.SummarizedContent{SummaryJapanese: "s"}, nil
	}).AnyTimes()
	summaryRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	jobRepo.EXPECT().CompleteJobWithNotification(gomock.Any(), gomock.Any(), gomock.Any(), "u1", gomock.Any()).DoAndReturn(func(_ context.Context, _, _, _, id string) error {
		if id == "missed-2" {
			missed2DoneOnce.Do(func() { close(missed2Done) })
		}
		return nil
	}).AnyTimes()

	log := testJobHandlerLogger()
	worker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log)
	worker.SetConcurrency(2)
	h := &jobHandler{queueWorker: worker, logger: log, jobGroup: orchestrator.NewJobGroup(ctx, log), batchSize: 2, lastBatchSweep: time.Now(), batchSweepForceInterval: 30 * time.Minute}
	require.NoError(t, h.processSummarizationBatch(ctx))

	done := make(chan error, 1)
	go func() { done <- h.processQueueTick(ctx) }()

	select {
	case <-missed2Done:
	case <-time.After(1500 * time.Millisecond):
		closeRelease()
		<-done
		t.Fatal("missed-2 was not processed while straggler-1 was in flight: worker exited despite pending rows")
	}

	closeRelease()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for processQueueTick to finish")
	}
}
