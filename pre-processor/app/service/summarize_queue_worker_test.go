package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"pre-processor/domain"
	"pre-processor/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- Local mocks for queue worker context cancellation tests ---

// stubJobRepo returns a fixed set of pending jobs.
type stubJobRepo struct {
	repository.SummarizeJobRepository
	jobs        []*domain.SummarizeJob
	cancelOnGet bool // cancel context after returning jobs
	cancelFunc  context.CancelFunc
	updateCalls int
	getErr      error // error to return from GetPendingJobs
}

func (m *stubJobRepo) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.cancelOnGet && m.cancelFunc != nil {
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
	jobID      string
	status     domain.SummarizeJobStatus
	summary    string
	errorMsg   string
}

func (m *stubJobRepoTracking) GetPendingJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
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

		// Should have 2 UpdateJobStatus calls: running + completed
		assert.Equal(t, 2, len(jobRepo.updateCalls), "should have running + completed status updates")

		// Second call should be completed with skip reason
		completedCall := jobRepo.updateCalls[1]
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
			jobs:        jobs,
			cancelOnGet: true,
			cancelFunc:  cancel,
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
