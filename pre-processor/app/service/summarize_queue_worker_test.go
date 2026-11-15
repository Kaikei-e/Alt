package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noRefill(context.Context) (bool, error) {
	return false, nil
}

// SetIdlePollIntervalForTest overrides the unexported idlePollInterval field for fast tests.
func SetIdlePollIntervalForTest(w *SummarizeQueueWorker, d time.Duration) {
	w.idlePollInterval = d
}

// --- Local mocks for queue worker context cancellation tests ---

// stubJobRepo returns a fixed set of pending jobs.
type stubJobRepo struct {
	repository.SummarizeJobRepository
	mu              sync.Mutex
	jobs            []*domain.SummarizeJob
	cancelOnDequeue bool // cancel context after returning jobs
	cancelFunc      context.CancelFunc
	updateCalls     int
	dequeueCalls    int
	getErr          error // error to return from DequeueJobs
}

func (m *stubJobRepo) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.jobs, nil
}

func (m *stubJobRepo) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dequeueCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.cancelOnDequeue && m.cancelFunc != nil {
		m.cancelFunc()
	}
	jobs := m.jobs
	m.jobs = nil
	return jobs, nil
}

func (m *stubJobRepo) UpdateJobStatus(_ context.Context, _ string, _ domain.SummarizeJobStatus, _ string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	return nil
}

func (m *stubJobRepo) RecoverStuckJobs(_ context.Context) (int64, error) {
	return 0, nil
}

// stubArticleRepoForWorker returns a fixed article for FindByID. A real
// repository backed by a DB connection pool serializes concurrent reads
// safely; this stub is exercised directly by ProcessQueue's worker pool (see
// TestSummarizeQueueWorker_ProcessQueue_UsesConfiguredConcurrency), so
// findCalls needs its own mutex to avoid a data race that has nothing to do
// with the production code under test.
type stubArticleRepoForWorker struct {
	repository.ArticleRepository
	mu        sync.Mutex
	findCalls int
}

func (m *stubArticleRepoForWorker) FindByID(_ context.Context, _ string) (*domain.Article, error) {
	m.mu.Lock()
	m.findCalls++
	m.mu.Unlock()
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

// stubSummaryRepoForWorker tracks calls to Create. A real repository backed
// by a DB connection pool serializes concurrent writes safely; this stub is
// exercised directly by ProcessQueue's worker pool (see
// TestSummarizeQueueWorker_ProcessQueue_UsesConfiguredConcurrency), so it
// needs its own mutex to avoid a data race on createCalls that has nothing
// to do with the production code under test.
type stubSummaryRepoForWorker struct {
	repository.SummaryRepository
	mu          sync.Mutex
	createCalls int
}

func (m *stubSummaryRepoForWorker) Create(_ context.Context, _ *domain.ArticleSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return nil
}

func (m *stubSummaryRepoForWorker) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
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

// stubAPIRepoContentNotProcessable returns ErrContentNotProcessable for SummarizeArticle.
type stubAPIRepoContentNotProcessable struct {
	repository.ExternalAPIRepository
	summarizeCalls int
}

func (m *stubAPIRepoContentNotProcessable) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	m.summarizeCalls++
	return nil, domain.ErrContentNotProcessable
}

// stubJobRepoTracking tracks UpdateJobStatus calls with their arguments. A
// real repository backed by a DB connection pool serializes concurrent
// writes safely; this stub is exercised directly by ProcessQueue's worker
// pool (see TestSummarizeQueueWorker_ProcessQueue_UsesConfiguredConcurrency),
// so appends to updateCalls need their own mutex to avoid a data race that
// has nothing to do with the production code under test.
type stubJobRepoTracking struct {
	repository.SummarizeJobRepository
	jobs         []*domain.SummarizeJob
	ignoreLimit  bool
	cancelFunc   context.CancelFunc
	mu           sync.Mutex
	updateCalls  []updateJobStatusCall
	recoverCalls int
}

type updateJobStatusCall struct {
	jobID    string
	status   domain.SummarizeJobStatus
	summary  string
	errorMsg string
	ctxErr   error
}

func (m *stubJobRepoTracking) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs, nil
}

func (m *stubJobRepoTracking) DequeueJobs(_ context.Context, limit int) ([]*domain.SummarizeJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	if len(m.jobs) == 0 {
		return nil, nil
	}
	if m.ignoreLimit || limit <= 0 || limit > len(m.jobs) {
		limit = len(m.jobs)
	}
	out := m.jobs[:limit]
	m.jobs = m.jobs[limit:]
	return out, nil
}

func (m *stubJobRepoTracking) UpdateJobStatus(ctx context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls = append(m.updateCalls, updateJobStatusCall{
		jobID:    jobID,
		status:   status,
		summary:  summary,
		errorMsg: errorMsg,
		ctxErr:   ctx.Err(),
	})
	return nil
}

// CompleteJobWithNotification records the same completion fact as
// UpdateJobStatus does, since from this stub's point of view both are "the job
// was marked completed" — only the transactional guarantee differs.
func (m *stubJobRepoTracking) CompleteJobWithNotification(_ context.Context, jobID, summary, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls = append(m.updateCalls, updateJobStatusCall{
		jobID:   jobID,
		status:  domain.SummarizeJobStatusCompleted,
		summary: summary,
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
		)

		err := worker.ProcessQueue(ctx, noRefill)

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

	t.Run("should release remaining dequeued jobs back to pending when first job is overloaded", func(t *testing.T) {
		ctx := context.Background()

		job1 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3}
		job2 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3}
		job3 := &domain.SummarizeJob{JobID: uuid.New(), ArticleID: "article-3", MaxRetries: 3}
		jobs := []*domain.SummarizeJob{job1, job2, job3}

		jobRepo := &stubJobRepoTracking{jobs: jobs, ignoreLimit: true}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoOverloaded{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
		)

		err := worker.ProcessQueue(ctx, noRefill)
		assert.ErrorIs(t, err, domain.ErrServiceOverloaded)
		assert.Equal(t, 1, apiRepo.summarizeCalls)

		if assert.Len(t, jobRepo.updateCalls, 3, "first job fails and the 2 unprocessed jobs must be released back to pending") {
			assert.Equal(t, job1.JobID.String(), jobRepo.updateCalls[0].jobID)
			assert.Equal(t, domain.SummarizeJobStatusFailed, jobRepo.updateCalls[0].status)

			assert.Equal(t, job2.JobID.String(), jobRepo.updateCalls[1].jobID)
			assert.Equal(t, domain.SummarizeJobStatusPending, jobRepo.updateCalls[1].status)

			assert.Equal(t, job3.JobID.String(), jobRepo.updateCalls[2].jobID)
			assert.Equal(t, domain.SummarizeJobStatusPending, jobRepo.updateCalls[2].status)
		}
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
		)

		err := worker.ProcessQueue(ctx, noRefill)

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
		)

		err := worker.ProcessQueue(ctx, noRefill)

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
		)

		err := worker.ProcessQueue(ctx, noRefill)
		assert.NoError(t, err)

		// No jobs should be processed because context was canceled after DequeueJobs,
		// and all dequeued jobs must be released back to pending.
		assert.Equal(t, 0, articleRepo.findCalls,
			"no articles should be fetched when context is canceled before processing jobs")
		assert.Equal(t, len(jobs), jobRepo.updateCalls,
			"dequeued jobs should be released back to pending when context is canceled")
	})

	t.Run("should release dequeued jobs with a valid context when context is already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		jobs := []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1", MaxRetries: 3},
			{JobID: uuid.New(), ArticleID: "article-2", MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs, ignoreLimit: true, cancelFunc: cancel}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoForWorker{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(
			jobRepo,
			articleRepo,
			apiRepo,
			summaryRepo,
			testLogger(),
		)

		err := worker.ProcessQueue(ctx, noRefill)
		assert.NoError(t, err)
		assert.Equal(t, 0, articleRepo.findCalls)

		if assert.Len(t, jobRepo.updateCalls, 2, "all dequeued jobs must be released back to pending") {
			for i, call := range jobRepo.updateCalls {
				assert.Equal(t, jobs[i].JobID.String(), call.jobID)
				assert.Equal(t, domain.SummarizeJobStatusPending, call.status)
				assert.NoError(t, call.ctxErr, "release must use a valid context even when worker ctx is canceled")
			}
		}
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
		)

		err := worker.ProcessQueue(ctx, noRefill)
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
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		err := worker.ProcessQueue(context.Background(), noRefill)
		assert.NoError(t, err)
		assert.Equal(t, 1, jobRepo.recoverCalls, "should call RecoverStuckJobs on first invocation")
	})

	t.Run("throttles RecoverStuckJobs to once per 5 minutes", func(t *testing.T) {
		jobRepo := &stubJobRepoWithRecovery{
			jobs:          []*domain.SummarizeJob{},
			recoverResult: 0,
		}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		// First call should trigger recovery
		_ = worker.ProcessQueue(context.Background(), noRefill)
		assert.Equal(t, 1, jobRepo.recoverCalls)

		// Second call immediately after should be throttled
		_ = worker.ProcessQueue(context.Background(), noRefill)
		assert.Equal(t, 1, jobRepo.recoverCalls, "should not call RecoverStuckJobs again within 5 minutes")
	})

	t.Run("continues processing even if recovery fails", func(t *testing.T) {
		jobRepo := &stubJobRepoWithRecovery{
			jobs:       []*domain.SummarizeJob{},
			recoverErr: fmt.Errorf("db error"),
		}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		err := worker.ProcessQueue(context.Background(), noRefill)
		assert.NoError(t, err, "ProcessQueue should succeed even if recovery fails")
		assert.Equal(t, 1, jobRepo.recoverCalls)
	})
}

func TestSummarizeQueueWorker_HasPendingJobs(t *testing.T) {
	t.Run("returns false when queue is empty", func(t *testing.T) {
		jobRepo := &stubJobRepo{jobs: []*domain.SummarizeJob{}}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		hasPending, err := worker.HasPendingJobs(context.Background())
		assert.NoError(t, err)
		assert.False(t, hasPending)
	})

	t.Run("returns true when queue has jobs", func(t *testing.T) {
		jobRepo := &stubJobRepo{jobs: []*domain.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "article-1"},
		}}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		hasPending, err := worker.HasPendingJobs(context.Background())
		assert.NoError(t, err)
		assert.True(t, hasPending)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		jobRepo := &stubJobRepo{getErr: fmt.Errorf("db connection failed")}
		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)
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

		worker := NewSummarizeQueueWorker(jobRepo, nil, nil, nil, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)
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
	jobs                 []*domain.SummarizeJob
	recentSuccessMap     map[string]bool
	inFlightMap          map[string]bool
	deadLetterMap        map[string]bool
	recentFailedMap      map[string]bool
	invalidatedCountMap  map[string]int
	markInFlightOnCreate bool
	createJobCalls       []string // article IDs passed to CreateJob
	dequeueCalls         int
	getErr               error
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

func (m *stubJobRepoWithEnqueue) HasInFlightJob(_ context.Context, articleID string, _ time.Time) (bool, error) {
	return m.inFlightMap[articleID], nil
}

func (m *stubJobRepoWithEnqueue) HasDeadLetterJob(_ context.Context, articleID string) (bool, error) {
	return m.deadLetterMap[articleID], nil
}

func (m *stubJobRepoWithEnqueue) HasRecentFailedJob(_ context.Context, articleID string, _ time.Time) (bool, error) {
	return m.recentFailedMap[articleID], nil
}

func (m *stubJobRepoWithEnqueue) CountInvalidatedCompletedJobs(_ context.Context, articleID string) (int, error) {
	return m.invalidatedCountMap[articleID], nil
}

func (m *stubJobRepoWithEnqueue) CreateJob(_ context.Context, articleID string) (string, error) {
	m.createJobCalls = append(m.createJobCalls, articleID)
	if m.markInFlightOnCreate {
		if m.inFlightMap == nil {
			m.inFlightMap = make(map[string]bool)
		}
		m.inFlightMap[articleID] = true
	}
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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, nil, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		assert.NoError(t, err)
		assert.True(t, result.HasMore)
	})
}

type pagedFindResponse struct {
	articles []*domain.Article
	cursor   *domain.Cursor
	err      error
}

type stubArticleRepoPaged struct {
	repository.ArticleRepository
	pages       map[string][]pagedFindResponse // keyed by cursor LastID ("" for nil); pops head if len > 1
	pageFunc    func(call int, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error)
	cursorOrder []*domain.Cursor
	findCalls   int
}

func (m *stubArticleRepoPaged) FindForSummarization(_ context.Context, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error) {
	m.findCalls++
	m.cursorOrder = append(m.cursorOrder, cursor)
	if m.pageFunc != nil {
		return m.pageFunc(m.findCalls, cursor, limit)
	}
	key := ""
	if cursor != nil {
		key = cursor.LastID
	}
	queue := m.pages[key]
	if len(queue) == 0 {
		return nil, nil, nil
	}
	resp := queue[0]
	if len(queue) > 1 {
		m.pages[key] = queue[1:]
	}
	return resp.articles, resp.cursor, resp.err
}

func (m *stubArticleRepoPaged) FindByID(_ context.Context, id string) (*domain.Article, error) {
	return &domain.Article{ID: id, Title: "Test", Content: "Test content"}, nil
}

func makeTestArticles(prefix string, count int) []*domain.Article {
	articles := make([]*domain.Article, count)
	for i := range count {
		articles[i] = &domain.Article{ID: fmt.Sprintf("%s-%d", prefix, i+1), Title: "Title"}
	}
	return articles
}

func TestEnqueueUnsummarizedBatch_SkipsRejectedPagesAndEnqueuesLaterEligible(t *testing.T) {
	t.Run("continues past guard-rejected pages and enqueues eligible articles in one call", func(t *testing.T) {
		ctx := context.Background()
		p1 := makeTestArticles("p1", 10)
		p2 := makeTestArticles("p2", 10)
		p3 := makeTestArticles("p3", 5)

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"":      {{articles: p1, cursor: &domain.Cursor{LastID: "p1-10"}}},
				"p1-10": {{articles: p2, cursor: &domain.Cursor{LastID: "p2-10"}}},
				"p2-10": {{articles: p3, cursor: nil}},
			},
		}
		invalidated := make(map[string]int)
		for _, a := range p1 {
			invalidated[a.ID] = 3
		}
		for _, a := range p2 {
			invalidated[a.ID] = 3
		}

		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap:    map[string]bool{},
			invalidatedCountMap: invalidated,
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)

		require.NoError(t, err)
		assert.Equal(t, 25, result.Found)
		assert.Equal(t, 5, result.Enqueued)
		assert.Equal(t, 20, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.False(t, result.HasMore)
		assert.Equal(t, 3, articleRepo.findCalls)
		assert.Equal(t, []string{"p3-1", "p3-2", "p3-3", "p3-4", "p3-5"}, jobRepo.createJobCalls)

		logs := logBuf.String()
		assert.Contains(t, logs, "batch enqueue completed")
		assert.Contains(t, logs, "pages=3")
		assert.Contains(t, logs, "low_quality_retries_exhausted:20")
		assert.NotContains(t, logs, "batch enqueue skipped by guard")
	})
}

func TestEnqueueUnsummarizedBatch_NeverExceedsBatchSizeAndResumesMidPage(t *testing.T) {
	t.Run("stops at batchSize mid-page and next call examines remaining articles", func(t *testing.T) {
		ctx := context.Background()
		p1 := makeTestArticles("p1", 4)
		p2 := makeTestArticles("p2", 4)

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"":     {{articles: p1, cursor: &domain.Cursor{LastID: "p1-4"}}},
				"p1-4": {{articles: p2, cursor: nil}},
			},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{},
			invalidatedCountMap: map[string]int{
				"p1-1": 3,
				"p1-2": 3,
			},
			markInFlightOnCreate: true,
		}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		result1, err := worker.EnqueueUnsummarizedBatch(ctx, 4)
		require.NoError(t, err)
		assert.Equal(t, 4, result1.Enqueued)
		assert.True(t, result1.HasMore, "unexamined articles remain on page 2")
		assert.Equal(t, []string{"p1-3", "p1-4", "p2-1", "p2-2"}, jobRepo.createJobCalls)

		result2, err := worker.EnqueueUnsummarizedBatch(ctx, 4)
		require.NoError(t, err)
		assert.Equal(t, 2, result2.Enqueued)
		assert.False(t, result2.HasMore, "all articles on final page now examined")
		assert.Equal(t, []string{"p1-3", "p1-4", "p2-1", "p2-2", "p2-3", "p2-4"}, jobRepo.createJobCalls)
		require.GreaterOrEqual(t, len(articleRepo.cursorOrder), 3)
		require.NotNil(t, articleRepo.cursorOrder[2])
		assert.Equal(t, "p1-4", articleRepo.cursorOrder[2].LastID, "second sweep should resume at page 2 cursor")
	})
}

func TestEnqueueUnsummarizedBatch_AggregatesToEndOfList(t *testing.T) {
	t.Run("aggregates counts across multiple pages until end of list", func(t *testing.T) {
		ctx := context.Background()
		p1 := makeTestArticles("p1", 5)
		p2 := makeTestArticles("p2", 3)

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"":     {{articles: p1, cursor: &domain.Cursor{LastID: "p1-5"}}},
				"p1-5": {{articles: p2, cursor: nil}},
			},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{
				"p1-1": true,
				"p2-1": true,
			},
			deadLetterMap: map[string]bool{
				"p1-2": true,
			},
			invalidatedCountMap: map[string]int{
				"p1-3": 3,
				"p2-2": 4,
			},
		}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 5)
		require.NoError(t, err)
		assert.Equal(t, 8, result.Found)
		assert.Equal(t, 3, result.Enqueued)
		assert.Equal(t, 5, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.False(t, result.HasMore)
		assert.Equal(t, []string{"p1-4", "p1-5", "p2-3"}, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_StopsAtPageBoundWithWarn(t *testing.T) {
	t.Run("stops at page-count safety bound while more remain and logs WARN", func(t *testing.T) {
		ctx := context.Background()
		const testCap = 500
		invalidated := make(map[string]int)

		articleRepo := &stubArticleRepoPaged{
			pageFunc: func(call int, _ *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error) {
				if call > testCap {
					return nil, nil, fmt.Errorf("exceeded test safety cap %d", testCap)
				}
				arts := makeTestArticles(fmt.Sprintf("pg%d", call), limit)
				for _, a := range arts {
					invalidated[a.ID] = 3
				}
				return arts, &domain.Cursor{LastID: arts[len(arts)-1].ID}, nil
			},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap:    map[string]bool{},
			invalidatedCountMap: invalidated,
		}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(ctx, 10)
		require.NoError(t, err)
		assert.True(t, result.HasMore)
		assert.GreaterOrEqual(t, result.Found, 1000, "bound must cover at least 1000 articles at batchSize 10")
		assert.Equal(t, 0, result.Enqueued)
		assert.Equal(t, result.Found, result.Skipped)
		assert.Less(t, articleRepo.findCalls, testCap)

		logs := logBuf.String()
		assert.Contains(t, logs, "level=WARN")
		assert.Contains(t, logs, fmt.Sprintf("found=%d", result.Found))
		assert.Contains(t, logs, fmt.Sprintf("skipped=%d", result.Skipped))
	})
}

func TestEnqueueUnsummarizedBatch_LaterPageErrorKeepsCursor(t *testing.T) {
	t.Run("returns wrapped error on later page and keeps cursor reached so far", func(t *testing.T) {
		ctx := context.Background()
		p1 := makeTestArticles("p1", 5)
		p2 := makeTestArticles("p2", 2)
		pageErr := errors.New("backend timeout")

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"": {{articles: p1, cursor: &domain.Cursor{LastID: "p1-5"}}},
				"p1-5": {
					{err: pageErr},
					{articles: p2, cursor: nil},
				},
			},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{},
			invalidatedCountMap: map[string]int{
				"p1-1": 3,
				"p1-2": 3,
				"p1-3": 3,
			},
		}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		_, err := worker.EnqueueUnsummarizedBatch(ctx, 5)
		require.Error(t, err)
		assert.ErrorIs(t, err, pageErr)

		result2, err := worker.EnqueueUnsummarizedBatch(ctx, 5)
		require.NoError(t, err)
		assert.Equal(t, 2, result2.Enqueued)
		assert.False(t, result2.HasMore)
		require.GreaterOrEqual(t, len(articleRepo.cursorOrder), 3)
		require.NotNil(t, articleRepo.cursorOrder[2])
		assert.Equal(t, "p1-5", articleRepo.cursorOrder[2].LastID)
		assert.Equal(t, []string{"p1-4", "p1-5", "p2-1", "p2-2"}, jobRepo.createJobCalls)
	})
}

func TestEnqueueUnsummarizedBatch_ContextCancelledMidPageKeepsCursor(t *testing.T) {
	t.Run("stops on context cancellation mid-page and keeps cursor before partial page", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		p1 := makeTestArticles("p1", 3)
		p2 := makeTestArticles("p2", 3)

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"":     {{articles: p1, cursor: &domain.Cursor{LastID: "p1-3"}}},
				"p1-3": {{articles: p2, cursor: nil}},
			},
		}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoCancellingOnArticle{
			stubJobRepoWithEnqueue: stubJobRepoWithEnqueue{
				recentSuccessMap: map[string]bool{},
				invalidatedCountMap: map[string]int{
					"p1-1": 3,
					"p1-2": 3,
				},
				markInFlightOnCreate: true,
			},
			cancelOnID: "p2-1",
			cancelFunc: cancel,
		}
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		result1, err := worker.EnqueueUnsummarizedBatch(ctx, 5)
		require.NoError(t, err)
		assert.Equal(t, 2, result1.Enqueued)
		assert.True(t, result1.HasMore)

		result2, err := worker.EnqueueUnsummarizedBatch(context.Background(), 5)
		require.NoError(t, err)
		assert.Equal(t, 2, result2.Enqueued)
		assert.False(t, result2.HasMore)
		require.GreaterOrEqual(t, len(articleRepo.cursorOrder), 3)
		require.NotNil(t, articleRepo.cursorOrder[2])
		assert.Equal(t, "p1-3", articleRepo.cursorOrder[2].LastID)
	})
}

type stubJobRepoCancellingOnArticle struct {
	stubJobRepoWithEnqueue
	cancelOnID string
	cancelFunc context.CancelFunc
}

func (m *stubJobRepoCancellingOnArticle) CreateJob(ctx context.Context, articleID string) (string, error) {
	id, err := m.stubJobRepoWithEnqueue.CreateJob(ctx, articleID)
	if articleID == m.cancelOnID && m.cancelFunc != nil {
		m.cancelFunc()
	}
	return id, err
}

func TestEnqueueUnsummarizedBatch_PerArticleSkipLoggedAtDebug(t *testing.T) {
	t.Run("logs per-article guard skip at DEBUG and per-reason counts at INFO", func(t *testing.T) {
		ctx := context.Background()
		articles := makeTestArticles("a", 2)

		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{"a-1": true}}
		jobRepo := &stubJobRepoWithEnqueue{recentSuccessMap: map[string]bool{}}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		_, err := worker.EnqueueUnsummarizedBatch(ctx, 10)
		require.NoError(t, err)

		logs := logBuf.String()
		assert.Contains(t, logs, "level=DEBUG msg=\"batch enqueue skipped by guard\"")
		assert.Contains(t, logs, "level=INFO msg=\"batch enqueue completed\"")
		assert.Contains(t, logs, "pages=1")
		assert.Contains(t, logs, "summary_exists:1")
	})
}

func TestEnqueueUnsummarizedBatch_ContextCancelledBeforeFirstPage(t *testing.T) {
	t.Run("returns context error without resetting stored cursor when cancelled before first page", func(t *testing.T) {
		p1 := makeTestArticles("p1", 2)
		p2 := makeTestArticles("p2", 2)

		articleRepo := &stubArticleRepoPaged{
			pages: map[string][]pagedFindResponse{
				"":     {{articles: p1, cursor: &domain.Cursor{LastID: "p1-2"}}},
				"p1-2": {{articles: p2, cursor: nil}},
			},
		}
		jobRepo := &stubJobRepoWithEnqueue{recentSuccessMap: map[string]bool{}}
		summaryRepo := &stubSummaryRepoWithExists{existsMap: map[string]bool{}}
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, testLogger())

		// Advance stored cursor to p1-2 on an initial sweep.
		result1, err := worker.EnqueueUnsummarizedBatch(context.Background(), 2)
		require.NoError(t, err)
		require.True(t, result1.HasMore)

		// Pre-cancelled context before fetching the next sweep's first page.
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		result2, err := worker.EnqueueUnsummarizedBatch(cancelledCtx, 2)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result2)
		assert.Equal(t, 1, articleRepo.findCalls, "cancelled pre-first-page sweep must not call FindForSummarization")

		// Subsequent non-cancelled sweep must still resume from the stored cursor p1-2.
		result3, err := worker.EnqueueUnsummarizedBatch(context.Background(), 2)
		require.NoError(t, err)
		assert.Equal(t, 2, result3.Enqueued)
		assert.False(t, result3.HasMore)
		require.Len(t, articleRepo.cursorOrder, 2)
		require.NotNil(t, articleRepo.cursorOrder[1])
		assert.Equal(t, "p1-2", articleRepo.cursorOrder[1].LastID)
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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

		assert.NoError(t, err)
		assert.Equal(t, 1, summaryRepo.createCalls, "should save placeholder summary")
		assert.NotNil(t, summaryRepo.lastSummary, "should have saved a summary")
		assert.Equal(t, "article-short", summaryRepo.lastSummary.ArticleID)
		assert.Equal(t, "user-1", summaryRepo.lastSummary.UserID)
		assert.Equal(t, "本文が短すぎるため要約できませんでした。", summaryRepo.lastSummary.SummaryJapanese,
			"should save the known placeholder for short content")
		assert.False(t, summaryRepo.lastSummary.SkipVersioning, "too-short placeholder must preserve default versioning")
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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

		assert.NoError(t, err, "should not return error for long content")
		assert.Equal(t, 1, summaryRepo.createCalls, "should save placeholder summary")
		assert.NotNil(t, summaryRepo.lastSummary)
		assert.Equal(t, "article-long", summaryRepo.lastSummary.ArticleID)
		assert.Equal(t, "本文が長すぎるため要約できませんでした。", summaryRepo.lastSummary.SummaryJapanese,
			"should save the known placeholder for long content")
		assert.False(t, summaryRepo.lastSummary.SkipVersioning, "too-long placeholder must preserve default versioning")

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		err := worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)

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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)

		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusFailed, jobRepo.updateCalls[0].status,
			"should mark as Failed (repository promotes retry-exhausted jobs to the 'failed' terminal status, not dead_letter) when no persisted summary")
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

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())
		worker.SetConcurrency(3)
		worker.idlePollInterval = 10 * time.Millisecond

		err := worker.ProcessQueue(context.Background(), noRefill)

		assert.NoError(t, err)
		assert.Equal(t, int32(3), atomic.LoadInt32(&apiRepo.calls))
		assert.Equal(t, int32(3), atomic.LoadInt32(&apiRepo.maxInFlight),
			"expected worker to utilize configured concurrency")
	})
}

func TestSummarizeQueueWorker_ProcessQueue_ContentNotProcessable(t *testing.T) {
	t.Run("job with RetryCount 0 retries via generic error transition instead of immediate dead_letter", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		// Initial attempt: retry_count=0, max_retries=3 -> should be retried via generic failure transition
		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-1", RetryCount: 0, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentNotProcessable{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)

		assert.Equal(t, 1, apiRepo.summarizeCalls, "should attempt summarization")
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusFailed, jobRepo.updateCalls[0].status,
			"should update status using retry transition (Failed), NOT dead_letter")
	})

	t.Run("job with exhausted retries moves to dead_letter", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		// Final attempt: retry_count=2, max_retries=3 -> exhausted retries should dead_letter
		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-1", RetryCount: 2, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentNotProcessable{}
		summaryRepo := &stubSummaryRepoForWorker{}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)

		assert.Equal(t, 1, apiRepo.summarizeCalls, "should attempt summarization")
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusDeadLetter, jobRepo.updateCalls[0].status,
			"should mark job as dead_letter when retries are exhausted")
	})

	t.Run("exhausted retries with summary already persisted marks Completed instead of dead_letter", func(t *testing.T) {
		ctx := context.Background()
		jobID := uuid.New()

		// Exhausted retries, but summary already exists upstream (e.g. concurrent attempt)
		jobs := []*domain.SummarizeJob{
			{JobID: jobID, ArticleID: "article-persisted", RetryCount: 2, MaxRetries: 3},
		}

		jobRepo := &stubJobRepoTracking{jobs: jobs}
		articleRepo := &stubArticleRepoForWorker{}
		apiRepo := &stubAPIRepoContentNotProcessable{}
		summaryRepo := &stubSummaryRepoExists{existsMap: map[string]bool{"article-persisted": true}}

		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, testLogger())

		_ = worker.ProcessQueue(ctx, noRefill)

		assert.Equal(t, 1, apiRepo.summarizeCalls, "should attempt summarization")
		assert.GreaterOrEqual(t, summaryRepo.existsCalls, 1, "should recheck summaryRepo.Exists before dead_letter")
		assert.Equal(t, 1, len(jobRepo.updateCalls), "should update status exactly once")
		assert.Equal(t, domain.SummarizeJobStatusCompleted, jobRepo.updateCalls[0].status,
			"should mark as Completed when upstream summary is already persisted")
	})
}

type stubSummaryRepoTerminalTracking struct {
	repository.SummaryRepository
	existsMap    map[string]bool
	createErrMap map[string]error
	createdItems []*domain.ArticleSummary
}

func (m *stubSummaryRepoTerminalTracking) Exists(_ context.Context, articleID string) (bool, error) {
	return m.existsMap[articleID], nil
}

func (m *stubSummaryRepoTerminalTracking) Create(_ context.Context, s *domain.ArticleSummary) error {
	if err := m.createErrMap[s.ArticleID]; err != nil {
		return err
	}
	m.createdItems = append(m.createdItems, s)
	return nil
}

func TestEnqueueUnsummarizedBatch_TerminalPlaceholders(t *testing.T) {
	t.Run("saves dead-letter placeholder once with SkipVersioning=true and no job created", func(t *testing.T) {
		articles := []*domain.Article{
			{ID: "art-dead", UserID: "user-1", Title: "Dead Letter Article"},
		}
		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoTerminalTracking{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			deadLetterMap: map[string]bool{"art-dead": true},
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(context.Background(), 10)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Found)
		assert.Equal(t, 0, result.Enqueued)
		assert.Equal(t, 1, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.Empty(t, jobRepo.createJobCalls)

		require.Len(t, summaryRepo.createdItems, 1)
		saved := summaryRepo.createdItems[0]
		assert.Equal(t, "art-dead", saved.ArticleID)
		assert.Equal(t, "user-1", saved.UserID)
		assert.Equal(t, "Dead Letter Article", saved.ArticleTitle)
		assert.Equal(t, domain.PlaceholderDeadLetter, saved.SummaryJapanese)
		assert.True(t, saved.SkipVersioning)

		logs := logBuf.String()
		assert.Contains(t, logs, "batch enqueue completed")
		assert.Contains(t, logs, "dead_letter_exists:1")
		assert.Contains(t, logs, "terminal_placeholders=1")
	})

	t.Run("saves low-quality-exhausted placeholder once with SkipVersioning=true and no job created", func(t *testing.T) {
		articles := []*domain.Article{
			{ID: "art-lq", UserID: "user-2", Title: "Low Quality Article"},
		}
		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoTerminalTracking{existsMap: map[string]bool{}}
		jobRepo := &stubJobRepoWithEnqueue{
			invalidatedCountMap: map[string]int{"art-lq": 3},
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(context.Background(), 10)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Found)
		assert.Equal(t, 0, result.Enqueued)
		assert.Equal(t, 1, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.Empty(t, jobRepo.createJobCalls)

		require.Len(t, summaryRepo.createdItems, 1)
		saved := summaryRepo.createdItems[0]
		assert.Equal(t, "art-lq", saved.ArticleID)
		assert.Equal(t, "user-2", saved.UserID)
		assert.Equal(t, "Low Quality Article", saved.ArticleTitle)
		assert.Equal(t, domain.PlaceholderLowQualityExhausted, saved.SummaryJapanese)
		assert.True(t, saved.SkipVersioning)

		logs := logBuf.String()
		assert.Contains(t, logs, "batch enqueue completed")
		assert.Contains(t, logs, "low_quality_retries_exhausted:1")
		assert.Contains(t, logs, "terminal_placeholders=1")
	})

	t.Run("does not save placeholders for non-terminal skip reasons", func(t *testing.T) {
		articles := []*domain.Article{
			{ID: "art-exists", UserID: "u1", Title: "Exists"},
			{ID: "art-recent-ok", UserID: "u1", Title: "Recent Success"},
			{ID: "art-inflight", UserID: "u1", Title: "In Flight"},
			{ID: "art-cooldown", UserID: "u1", Title: "Cooldown"},
		}
		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoTerminalTracking{
			existsMap: map[string]bool{"art-exists": true},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			recentSuccessMap: map[string]bool{"art-recent-ok": true},
			inFlightMap:      map[string]bool{"art-inflight": true},
			recentFailedMap:  map[string]bool{"art-cooldown": true},
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(context.Background(), 10)
		require.NoError(t, err)
		assert.Equal(t, 4, result.Found)
		assert.Equal(t, 0, result.Enqueued)
		assert.Equal(t, 4, result.Skipped)
		assert.Equal(t, 0, result.Errors)
		assert.Empty(t, summaryRepo.createdItems)
		assert.Empty(t, jobRepo.createJobCalls)

		logs := logBuf.String()
		assert.Contains(t, logs, "batch enqueue completed")
		assert.Contains(t, logs, "terminal_placeholders=0")
	})

	t.Run("counts failing placeholder save in Errors and continues sweep", func(t *testing.T) {
		articles := []*domain.Article{
			{ID: "art-dead-fail", UserID: "u1", Title: "Dead Fail"},
			{ID: "art-lq-ok", UserID: "u2", Title: "LQ Ok"},
			{ID: "art-eligible", UserID: "u3", Title: "Eligible"},
		}
		articleRepo := &stubArticleRepoWithFind{articles: articles}
		summaryRepo := &stubSummaryRepoTerminalTracking{
			existsMap:    map[string]bool{},
			createErrMap: map[string]error{"art-dead-fail": errors.New("rpc unavailable")},
		}
		jobRepo := &stubJobRepoWithEnqueue{
			deadLetterMap:       map[string]bool{"art-dead-fail": true},
			invalidatedCountMap: map[string]int{"art-lq-ok": 3},
		}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		worker := NewSummarizeQueueWorker(jobRepo, articleRepo, nil, summaryRepo, logger)

		result, err := worker.EnqueueUnsummarizedBatch(context.Background(), 10)
		require.NoError(t, err)
		assert.Equal(t, 3, result.Found)
		assert.Equal(t, 1, result.Enqueued)
		assert.Equal(t, 2, result.Skipped)
		assert.Equal(t, 1, result.Errors)
		assert.Equal(t, []string{"art-eligible"}, jobRepo.createJobCalls)

		require.Len(t, summaryRepo.createdItems, 1)
		assert.Equal(t, "art-lq-ok", summaryRepo.createdItems[0].ArticleID)
		assert.True(t, summaryRepo.createdItems[0].SkipVersioning)

		logs := logBuf.String()
		assert.Contains(t, logs, "level=ERROR")
		assert.Contains(t, logs, "batch enqueue completed")
		assert.Contains(t, logs, "terminal_placeholders=1")
	})
}
