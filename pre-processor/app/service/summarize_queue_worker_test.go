package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- Local mocks for queue worker context cancellation tests ---

// stubJobRepo returns a fixed set of pending jobs.
type stubJobRepo struct {
	repository.SummarizeJobRepository
	jobs            []*domain.SummarizeJob
	cancelOnDequeue bool // cancel context after returning jobs
	cancelFunc      context.CancelFunc
	updateCalls     int
	dequeueCalls    int
	getErr          error // error to return from DequeueJobs
}

func (m *stubJobRepo) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepo) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	m.dequeueCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.cancelOnDequeue && m.cancelFunc != nil {
		m.cancelFunc()
	}
	return m.jobs, nil
}

func (m *stubJobRepo) UpdateJobStatus(_ context.Context, _ string, _ domain.SummarizeJobStatus, _ string, _ string) error {
	m.updateCalls++
	return nil
}

func (m *stubJobRepo) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

// stubArticleRepoForWorker returns a fixed article for FindByID.
type stubArticleRepoForWorker struct {
	repository.ArticleRepository
	findCalls int
}

func (m *stubArticleRepoForWorker) FindByID(_ context.Context, _ string) (*domain.Article, error) {
	m.findCalls++
	return &domain.Article{
		ID:      "article-1",
		UserID:  "user-1",
		Title:   "Test Article",
		Content: "Test content for summarization",
	}, nil
}

// stubAPIRepoForWorker tracks calls to SummarizeArticle.
type stubAPIRepoForWorker struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoForWorker) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return &domain.SummarizedContent{SummaryJapanese: "テスト要約"}, nil
}

// stubSummaryRepoForWorker tracks calls to Create.
type stubSummaryRepoForWorker struct {
	repository.SummaryRepository
	createCalls int
}

func (m *stubSummaryRepoForWorker) Create(_ context.Context, _ *domain.ArticleSummary) error {
	m.createCalls++
	return nil
}

// stubAPIRepoOverloaded returns ErrServiceOverloaded for SummarizeArticle.
type stubAPIRepoOverloaded struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoOverloaded) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return nil, domain.ErrServiceOverloaded
}

// stubAPIRepoContentTooShort returns ErrContentTooShort for SummarizeArticle.
type stubAPIRepoContentTooShort struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoContentTooShort) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return nil, domain.ErrContentTooShort
}

// stubJobRepoTracking tracks UpdateJobStatus calls with their arguments.
type stubJobRepoTracking struct {
	repository.SummarizeJobRepository
	jobs         []*domain.SummarizeJob
	updateCalls  []updateJobStatusCall
	recoverCalls int
}

type updateJobStatusCall struct {
	jobID    string
	status   domain.SummarizeJobStatus
	summary  string
	errorMsg string
}

func (m *stubJobRepoTracking) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	return m.jobs, nil
}

func (m *stubJobRepoTracking) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	return m.jobs, nil
}

func (m *stubJobRepoTracking) UpdateJobStatus(_ context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMsg string) error {
	m.updateCalls = append(m.updateCalls, updateJobStatusCall{
		jobID:    jobID,
		status:   status,
		summary:  summary,
		errorMsg: errorMsg,
	})
	return nil
}

func (m *stubJobRepoTracking) RecoverStuckJobs(_ context.Context) (int64, error) {
	m.recoverCalls++
	return 0, nil
}

func TestSummarizeQueueWorker_ProcessQueue_ServiceOverloaded(t *testing.T) {
	t.Run("should return ErrServiceOverloaded and skip remaining jobs on 429", func(t *testing.T) {
		ctx := context.Background()

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-3", MaxRetries: 3},
		}

		jobRepo := &stubJobRepo{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoOverloaded{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
			10,
		)

		err := worker.ProcessQueue(ctx)

		// Should return ErrServiceOverloaded
		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrServiceOverloaded),
			"should return ErrServiceOverloaded, got: %v", err)

		// Only the first job should be attempted (then backoff kicks in)
		assert.Equal(t, 1, apiRepo.summarizeCalls,
			"should stop after first overloaded response")
		assert.Equal(t, 1, articleRepo.findCalls,
			"should only fetch article for the first job")
	})
}

func TestSummarizeQueueWorker_ProcessQueue_ContentTooShort(t *testing.T) {
	t.Run("should mark job as completed with skip reason when content is too short", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-short", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentTooShort{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
			10,
		)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err, "should not return error for short content skip")
		assert.Equal(t, 1, apiRepo.summarizeCalls, "should attempt summarization")

		// Should have 1 UpdateJobStatus call: completed only (running is handled by DequeueJobs)
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should have completed status update only")

		// The call should be completed with skip reason
		completedCall := jobRepo.updateCalls[0]
		assert.Equal(t, domain.SummarizeJobStatusCompleted, completedCall.status,
			"should mark as completed, not dead_letter")
		assert.Equal(t, "", completedCall.summary, "summary should be empty for skipped jobs")
		assert.Contains(t, completedCall.errorMsg, "skipped: content too short",
			"should include skip reason")
	})

	t.Run("should continue processing remaining jobs after short content skip", func(t *testing.T) {
		ctx := context.Background()

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-short", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-normal", MaxRetries: 3},
		}

		// Use a custom API repo that returns ErrContentTooShort for the first call, success for the second
		apiRepo := &stubAPIRepoContentTooShortThenSuccess{}
		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
			10,
		)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 2, apiRepo.summarizeCalls, "should attempt both jobs")
		assert.Equal(t, 2, articleRepo.findCalls, "should fetch both articles")
	})
}

// stubAPIRepoContentTooShortThenSuccess returns ErrContentTooShort on first call, success on subsequent calls.
type stubAPIRepoContentTooShortThenSuccess struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

type stubAPIRepoConcurrent struct {
	repository.ExternalAPIRepository
	delay       time.Duration
	inFlight    int32
	maxInFlight int32
	calls       int32
}

func (m *stubAPIRepoConcurrent) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	current := atomic.AddInt32(&m.inFlight, 1)
	atomic.AddInt32(&m.calls, 1)
	defer atomic.AddInt32(&m.inFlight, -1)

	for {
		observed := atomic.LoadInt32(&m.maxInFlight)
		if current <= observed {
			break
		}
		if atomic.CompareAndSwapInt32(&m.maxInFlight, observed, current) {
			break
		}
	}

	time.Sleep(m.delay)
	return &domain.SummarizedContent{SummaryJapanese: "テスト要約"}, nil
}

func (m *stubAPIRepoContentTooShortThenSuccess) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	if m.summarizeCalls == 1 {
		return nil, domain.ErrContentTooShort
	}
	return &domain.SummarizedContent{SummaryJapanese: "テスト要約"}, nil
}

func TestSummarizeQueueWorker_ProcessQueue_ContextCanceled(t *testing.T) {
	t.Run("should skip remaining jobs when context is canceled after fetching", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-3", MaxRetries: 3},
		}

		jobRepo := &stubJobRepo{
			jobs:            jobs,
			cancelOnDequeue: true,
			cancelFunc:      cancel,
		}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
			10,
		)

		err := worker.ProcessQueue(ctx)
		assert.NoError(t, err)

		// No jobs should be processed because context was canceled after GetPendingJobs
		assert.Equal(t, 0, articleRepo.findCalls,
			"no articles should be fetched when context is canceled before processing jobs")
		assert.Equal(t, 0, jobRepo.updateCalls,
			"no job status updates should occur when context is canceled")
	})

	t.Run("should process zero jobs when context is already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before processing

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
		}

		jobRepo := &stubJobRepo{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
			10,
		)

		err := worker.ProcessQueue(ctx)
		assert.NoError(t, err)

		assert.Equal(t, 0, articleRepo.findCalls,
			"no articles should be fetched when context is already canceled")
	})
}

// stubJobRepoWithRecovery tracks RecoverStuckJobs calls.
type stubJobRepoWithRecovery struct {
	repository.SummarizeJobRepository
	jobs          []*domain.SummarizeJob
	getErr        error
	recoverCalls  int
	recoverResult int64
	recoverErr    error
}

func (m *stubJobRepoWithRecovery) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoWithRecovery) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepoWithRecovery) UpdateJobStatus(_ context.Context, _ string, _ domain.SummarizeJobStatus, _ string, _ string) error {
	return nil
}

func (m *stubJobRepoWithRecovery) RecoverStuckJobs(_ context.Context) (int64, error) {
	m.recoverCalls++
	return m.recoverResult, m.recoverErr
}

func TestSummarizeQueueWorker_RecoverStuckJobs(t *testing.T) {
	t.Run("calls RecoverStuckJobs on first ProcessQueue invocation", func(t *testing.T) {
		jobRepo := &stubJobRepoWithRecovery{
			jobs:          []*domain.SummarizeJob{},
			recoverResult: 5,
		}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		err := worker.ProcessQueue(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, jobRepo.recoverCalls, "should call RecoverStuckJobs on first invocation")
	})

	t.Run("throttles RecoverStuckJobs to once per 5 minutes", func(t *testing.T) {
		jobRepo := &stubJobRepoWithRecovery{
			jobs:          []*domain.SummarizeJob{},
			recoverResult: 0,
		}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		// First call should trigger recovery
		_ = worker.ProcessQueue(context.Background())
		assert.Equal(t, 1, jobRepo.recoverCalls)

		// Second call immediately after should be throttled
		_ = worker.ProcessQueue(context.Background())
		assert.Equal(t, 1, jobRepo.recoverCalls, "should not call RecoverStuckJobs again within 5 minutes")
	})

	t.Run("continues processing even if recovery fails", func(t *testing.T) {
		jobRepo := &stubJobRepoWithRecovery{
			jobs:       []*domain.SummarizeJob{},
			recoverErr: fmt.Errorf("db error"),
		}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		err := worker.ProcessQueue(context.Background())
		assert.NoError(t, err, "ProcessQueue should succeed even if recovery fails")
		assert.Equal(t, 1, jobRepo.recoverCalls)
	})
}

func TestSummarizeQueueWorker_HasPendingJobs(t *testing.T) {
	t.Run("returns false when queue is empty", func(t *testing.T) {
		jobRepo := &stubJobRepo{jobs: []*domain.SummarizeJob{}}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		hasPending, err := worker.HasPendingJobs(context.Background())
		assert.NoError(t, err)
		assert.False(t, hasPending)
	})

	t.Run("returns true when queue has jobs", func(t *testing.T) {
		jobRepo := &stubJobRepo{jobs: []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1"},
		}}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		hasPending, err := worker.HasPendingJobs(context.Background())
		assert.NoError(t, err)
		assert.True(t, hasPending)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		jobRepo := &stubJobRepo{getErr: fmt.Errorf("db connection failed")}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		hasPending, err := worker.HasPendingJobs(context.Background())
		assert.Error(t, err)
		assert.False(t, hasPending)
		assert.Contains(t, err.Error(), "db connection failed")
	})
}

// stubSummaryRepoFailing always returns an error from Create.
type stubSummaryRepoFailing struct {
	repository.SummaryRepository
	createCalls int
	createErr   error
}

func (m *stubSummaryRepoFailing) Create(_ context.Context, _ *domain.ArticleSummary) error {
	m.createCalls++
	return m.createErr
}

func TestSummarizeQueueWorker_ProcessQueue_SummarySaveFailure(t *testing.T) {
	t.Run("should not mark job completed when summary save fails", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-1", Status: domain.SummarizeJobStatusRunning, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoFailing{createErr: fmt.Errorf("database connection lost")}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		// ProcessQueue does not propagate individual job errors (only ErrServiceOverloaded)
		assert.NoError(t, err)

		// summary save was attempted
		assert.Equal(t, 1, summaryRepo.createCalls, "should attempt to save summary")

		// Job should be marked as failed, NOT completed
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should have exactly one status update")
		failedCall := jobRepo.updateCalls[0]
		assert.Equal(t, domain.SummarizeJobStatusFailed, failedCall.status,
			"job should be marked as failed when summary save fails, not completed")
		assert.Contains(t, failedCall.errorMsg, "database connection lost",
			"error message should describe the save failure")
	})

	t.Run("should mark job completed when summary saves successfully", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-1", Status: domain.SummarizeJobStatusRunning, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 1, summaryRepo.createCalls, "should save summary")

		// Job should be marked as completed
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should have exactly one status update")
		completedCall := jobRepo.updateCalls[0]
		assert.Equal(t, domain.SummarizeJobStatusCompleted, completedCall.status,
			"job should be marked as completed when summary saves successfully")
	})
}

func TestSummarizeQueueWorker_ProcessQueue_AtomicDequeue(t *testing.T) {
	t.Run("uses DequeueJobs for atomic pending-to-running transition", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-1", Status: domain.SummarizeJobStatusRunning, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)
		assert.NoError(t, err)

		// Verify no UpdateJobStatus call with "running" status — dequeue handles that atomically
		for _, call := range jobRepo.updateCalls {
			assert.NotEqual(t, domain.SummarizeJobStatusRunning, call.status,
				"processJob should not separately transition to running; DequeueJobs does this atomically")
		}
	})

	t.Run("DequeueJobs is called instead of GetPendingJobs", func(t *testing.T) {
		ctx := context.Background()

		jobRepo := &stubJobRepo{jobs: []*domain.SummarizeJob{}}

		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger(), 10)

		_ = worker.ProcessQueue(ctx)
		assert.Equal(t, 1, jobRepo.dequeueCalls, "ProcessQueue should call DequeueJobs")
	})
}

// --- Stubs for EnqueueUnsummarizedBatch tests ---

// stubArticleRepoWithFind supports FindForSummarization in addition to FindByID.
type stubArticleRepoWithFind struct {
	repository.ArticleRepository
	articles  []*domain.Article
	cursor    *domain.Cursor
	findCalls int
}

func (m *stubArticleRepoWithFind) FindForSummarization(_ context.Context, _ *domain.Cursor, _ int) ([]*domain.Article, *domain.Cursor, error) {
	m.findCalls++
	return m.articles, m.cursor, nil
}

func (m *stubArticleRepoWithFind) FindByID(_ context.Context, _ string) (*domain.Article, error) {
	return &domain.Article{ID: "test", Title: "Test", Content: "Test content"}, nil
}

// stubSummaryRepoWithExists supports Exists for guard checks.
type stubSummaryRepoWithExists struct {
	repository.SummaryRepository
	existsMap   map[string]bool
	createCalls int
}

func (m *stubSummaryRepoWithExists) Exists(_ context.Context, articleID string) (bool, error) {
	return m.existsMap[articleID], nil
}

func (m *stubSummaryRepoWithExists) Create(_ context.Context, _ *domain.ArticleSummary) error {
	m.createCalls++
	return nil
}

// stubJobRepoWithEnqueue supports guard checks and CreateJob.
type stubJobRepoWithEnqueue struct {
	repository.SummarizeJobRepository
	jobs             []*domain.SummarizeJob
	recentSuccessMap map[string]bool
	createJobCalls   []string // article IDs passed to CreateJob
	dequeueCalls     int
	getErr           error
}

func (m *stubJobRepoWithEnqueue) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	return m.jobs, m.getErr
}

func (m *stubJobRepoWithEnqueue) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	m.dequeueCalls++
	return m.jobs, m.getErr
}

func (m *stubJobRepoWithEnqueue) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *stubJobRepoWithEnqueue) HasRecentSuccessfulJob(_ context.Context, articleID string, _ time.Time) (bool, error) {
	return m.recentSuccessMap[articleID], nil
}

func (m *stubJobRepoWithEnqueue) HasInFlightJob(_ context.Context, _ string, _ time.Time) (bool, error) {
	return false, nil
}

func (m *stubJobRepoWithEnqueue) CreateJob(_ context.Context, articleID string) (string, error) {
	m.createJobCalls = append(m.createJobCalls, articleID)
	return uuid.New().String(), nil
}

func (m *stubJobRepoWithEnqueue) UpdateJobStatus(_ context.Context, _ string, _ domain.SummarizeJobStatus, _ string, _ string) error {
	return nil
}

// stubJobRepoWithDuplicate returns empty string from CreateJob (duplicate detected).
type stubJobRepoWithDuplicate struct {
	stubJobRepoWithEnqueue
}

func (m *stubJobRepoWithDuplicate) CreateJob(_ context.Context, articleID string) (string, error) {
	m.createJobCalls = append(m.createJobCalls, articleID)
	return "", nil // empty string = duplicate, no error
}

func TestEnqueueUnsummarizedBatch_EnqueuesViaGuard(t *testing.T) {
	t.Run("enqueues articles that pass guard checks", func(t *testing.T) {
		ctx := context.Background()

		articles := []*domain.Article{
			{ID: "article-1", Title: "Title 1"},
			{ID: "article-2", Title: "Title 2"},
		}

		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{},
		}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.Equal(t, 2, result.Found)
		assert.Equal(t, 2, result.Enqueued)
		assert.Equal(t, 0, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.Equal(t, []string{"article-1", "article-2"}, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_SkipsSummaryExists(t *testing.T) {
	t.Run("skips articles that already have summaries", func(t *testing.T) {
		ctx := context.Background()

		articles := []*domain.Article{
			{ID: "article-1", Title: "Title 1"},
			{ID: "article-2", Title: "Title 2"},
		}

		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{
			"article-1": true, // has summary
		}}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{},
		}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.Equal(t, 2, result.Found)
		assert.Equal(t, 1, result.Enqueued)
		assert.Equal(t, 1, result.Skipped)
		assert.Equal(t, []string{"article-2"}, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_SkipsRecentSuccess(t *testing.T) {
	t.Run("skips articles with recent successful jobs", func(t *testing.T) {
		ctx := context.Background()

		articles := []*domain.Article{
			{ID: "article-1", Title: "Title 1"},
			{ID: "article-2", Title: "Title 2"},
		}

		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{
				"article-1": true, // has recent success
			},
		}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.Equal(t, 2, result.Found)
		assert.Equal(t, 1, result.Enqueued)
		assert.Equal(t, 1, result.Skipped)
		assert.Equal(t, []string{"article-2"}, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_NoArticles(t *testing.T) {
	t.Run("returns zero counts when no unsummarized articles exist", func(t *testing.T) {
		ctx := context.Background()

		articleRepo := &stubArticleRepoWithFind{articles: []*domain.Article{}}
		jobRepo := &stubJobRepoWithEnqueue{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, nil, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.Equal(t, 0, result.Found)
		assert.Equal(t, 0, result.Enqueued)
		assert.Equal(t, 0, result.Skipped)
		assert.False(t, result.HasMore)
		assert.Empty(t, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_DuplicateHandled(t *testing.T) {
	t.Run("handles duplicate gracefully when CreateJob returns empty string", func(t *testing.T) {
		ctx := context.Background()

		articles := []*domain.Article{
			{ID: "article-1", Title: "Title 1"},
		}

		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithDuplicate{
			stubJobRepoWithEnqueue: stubJobRepoWithEnqueue{
				recentSuccessMap: map[string]bool{},
			},
		}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.Equal(t, 1, result.Found)
		assert.Equal(t, 0, result.Enqueued) // duplicate, not counted as enqueued
		assert.Equal(t, 1, result.Skipped)  // counted as skipped (already in queue)
		assert.Equal(t, 0, result.Errors)
	})
}

func TestEnqueueUnsummarizedBatch_HasMore(t *testing.T) {
	t.Run("sets HasMore=true when cursor is returned", func(t *testing.T) {
		ctx := context.Background()

		articles := []*domain.Article{
			{ID: "article-1", Title: "Title 1"},
		}
		nextCursor := &domain.Cursor{LastID: "article-1"}

		articleRepo := &stubArticleRepoWithFind{articles: articles, cursor: nextCursor}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{},
		}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger(), 10)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.True(t, result.HasMore)
	})
}

// stubAPIRepoContentTooLong returns ErrContentTooLong for SummarizeArticle.
type stubAPIRepoContentTooLong struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoContentTooLong) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return nil, domain.ErrContentTooLong
}

// stubSummaryRepoTracking tracks Create calls and captures the saved summary.
type stubSummaryRepoTracking struct {
	repository.SummaryRepository
	createCalls   int
	lastSummary   *domain.ArticleSummary
	createErr     error
	existsResults map[string]bool
}

func (m *stubSummaryRepoTracking) Create(_ context.Context, summary *domain.ArticleSummary) error {
	m.createCalls++
	m.lastSummary = summary
	return m.createErr
}

func (m *stubSummaryRepoTracking) Exists(_ context.Context, articleID string) (bool, error) {
	if m.existsResults != nil {
		return m.existsResults[articleID], nil
	}
	return false, nil
}

func TestSummarizeQueueWorker_ProcessQueue_ContentTooShort_SavesPlaceholder(t *testing.T) {
	t.Run("should save placeholder summary when content is too short", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-short", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentTooShort{}
		summaryRepo := &stubSummaryRepoTracking{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 1, summaryRepo.createCalls, "should save placeholder summary")
		assert.NotNil(t, summaryRepo.lastSummary, "should have saved a summary")
		assert.Equal(t, "article-short", summaryRepo.lastSummary.ArticleID)
		assert.Equal(t, "user-1", summaryRepo.lastSummary.UserID)
		assert.Equal(t, "本文が短すぎるため要約できませんでした。", summaryRepo.lastSummary.SummaryJapanese,
			"should save the known placeholder for short content")
	})

	t.Run("should still complete job even if placeholder save fails", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-short", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentTooShort{}
		summaryRepo := &stubSummaryRepoTracking{createErr: fmt.Errorf("db error")}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err, "should not propagate placeholder save error")
		assert.Equal(t, 1, summaryRepo.createCalls, "should attempt to save placeholder")
		// Job should still be marked completed
		assert.Equal(t, 1, len(jobRepo.updateCalls))
		assert.Equal(t, domain.SummarizeJobStatusCompleted, jobRepo.updateCalls[0].status)
	})
}

func TestSummarizeQueueWorker_ProcessQueue_ContentTooLong_SavesPlaceholder(t *testing.T) {
	t.Run("should save placeholder summary and complete job when content is too long", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-long", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentTooLong{}
		summaryRepo := &stubSummaryRepoTracking{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err, "should not return error for long content")
		assert.Equal(t, 1, summaryRepo.createCalls, "should save placeholder summary")
		assert.NotNil(t, summaryRepo.lastSummary)
		assert.Equal(t, "article-long", summaryRepo.lastSummary.ArticleID)
		assert.Equal(t, "本文が長すぎるため要約できませんでした。", summaryRepo.lastSummary.SummaryJapanese,
			"should save the known placeholder for long content")

		// Should be marked completed, not dead_letter
		assert.Equal(t, 1, len(jobRepo.updateCalls))
		assert.Equal(t, domain.SummarizeJobStatusCompleted, jobRepo.updateCalls[0].status,
			"should mark as completed, not dead_letter")
		assert.Contains(t, jobRepo.updateCalls[0].errorMsg, "content too long")
	})

	t.Run("should continue processing remaining jobs after long content placeholder", func(t *testing.T) {
		ctx := context.Background()

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-long", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-normal", MaxRetries: 3},
		}

		// First call returns too long, second succeeds
		apiRepo := &stubAPIRepoContentTooLongThenSuccess{}
		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		summaryRepo := &stubSummaryRepoTracking{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 2, apiRepo.summarizeCalls, "should attempt both jobs")
		assert.Equal(t, 2, summaryRepo.createCalls, "should save summary for both (placeholder + real)")
	})
}

// stubAPIRepoContentTooLongThenSuccess returns ErrContentTooLong on first call, success on subsequent.
type stubAPIRepoContentTooLongThenSuccess struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoContentTooLongThenSuccess) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	if m.summarizeCalls == 1 {
		return nil, domain.ErrContentTooLong
	}
	return &domain.SummarizedContent{SummaryJapanese: "テスト要約"}, nil
}

// stubAPIRepoUpstreamBusy returns ErrUpstreamBusy for SummarizeArticle.
type stubAPIRepoUpstreamBusy struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoUpstreamBusy) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return nil, domain.ErrUpstreamBusy
}

// stubSummaryRepoExists reports Exists based on a preset map and tracks calls.
type stubSummaryRepoExists struct {
	repository.SummaryRepository
	existsMap    map[string]bool
	existsCalls  int
	createCalls  int
	createdItems []*domain.ArticleSummary
}

func (m *stubSummaryRepoExists) Exists(_ context.Context, articleID string) (bool, error) {
	m.existsCalls++
	return m.existsMap[articleID], nil
}

func (m *stubSummaryRepoExists) Create(_ context.Context, s *domain.ArticleSummary) error {
	m.createCalls++
	m.createdItems = append(m.createdItems, s)
	return nil
}

func TestSummarizeQueueWorker_ProcessQueue_UpstreamBusy_TriggersBackoff(t *testing.T) {
	t.Run("ErrUpstreamBusy from API halts the batch with ErrServiceOverloaded", func(t *testing.T) {
		ctx := context.Background()

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoUpstreamBusy{}
		summaryRepo := &stubSummaryRepoExists{existsMap: map[string]bool{}}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		err := worker.ProcessQueue(ctx)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrServiceOverloaded) || errors.Is(err, domain.ErrUpstreamBusy),
			"upstream busy should trigger backoff, got: %v", err)
		assert.Equal(t, 1, apiRepo.summarizeCalls,
			"should stop after first upstream-busy response")
	})
}

func TestSummarizeQueueWorker_ProcessQueue_DeadLetterRechecksSummaryExists(t *testing.T) {
	t.Run("marks job Completed instead of dead_letter when summary already exists upstream", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		// Final attempt: retry_count=2, max_retries=3 -> next failure would dead_letter
		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-saved", RetryCount: 2, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoUpstreamBusy{}
		summaryRepo := &stubSummaryRepoExists{existsMap: map[string]bool{"article-saved": true}}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		_ = worker.ProcessQueue(ctx)

		assert.GreaterOrEqual(t, summaryRepo.existsCalls, 1, "should recheck summaryRepo.Exists before dead_letter")
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusCompleted, jobRepo.updateCalls[0].status,
			"should mark as Completed when upstream summary is already persisted")
	})

	t.Run("falls through to Failed when summary does not exist", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-missing", RetryCount: 2, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoUpstreamBusy{}
		summaryRepo := &stubSummaryRepoExists{existsMap: map[string]bool{}}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)

		_ = worker.ProcessQueue(ctx)

		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusFailed, jobRepo.updateCalls[0].status,
			"should mark as Failed (repository promotes to dead_letter) when no persisted summary")
	})
}

func TestSummarizeQueueWorker_ProcessQueue_UsesConfiguredConcurrency(t *testing.T) {
	t.Run("processes jobs concurrently when worker concurrency is increased", func(t *testing.T) {
		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-3", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoConcurrent{delay: 40 * time.Millisecond}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger(), 10)
		worker.SetConcurrency(3)

		err := worker.ProcessQueue(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, int32(3), atomic.LoadInt32(&apiRepo.calls))
		assert.Equal(t, int32(3), atomic.LoadInt32(&apiRepo.maxInFlight),
			"expected worker to utilize configured concurrency")
	})
}
