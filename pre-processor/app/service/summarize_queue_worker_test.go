package service

import (
	"context"
	"testing"

	"pre-processor/models"
	"pre-processor/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- Local mocks for queue worker context cancellation tests ---

// stubJobRepo returns a fixed set of pending jobs.
type stubJobRepo struct {
	repository.SummarizeJobRepository
	jobs        []*models.SummarizeJob
	cancelOnGet bool // cancel context after returning jobs
	cancelFunc  context.CancelFunc
	updateCalls int
}

func (m *stubJobRepo) GetPendingJobs(_ context.Context, _ int) ([]*models.SummarizeJob, error) {
	if m.cancelOnGet && m.cancelFunc != nil {
		m.cancelFunc()
	}
	return m.jobs, nil
}

func (m *stubJobRepo) UpdateJobStatus(_ context.Context, _ string, _ models.SummarizeJobStatus, _ string, _ string) error {
	m.updateCalls++
	return nil
}

// stubArticleRepoForWorker returns a fixed article for FindByID.
type stubArticleRepoForWorker struct {
	repository.ArticleRepository
	findCalls int
}

func (m *stubArticleRepoForWorker) FindByID(_ context.Context, _ string) (*models.Article, error) {
	m.findCalls++
	return &models.Article{
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

func (m *stubAPIRepoForWorker) SummarizeArticle(_ context.Context, _ *models.Article, _ string) (*models.SummarizedContent, error) {
	m.summarizeCalls++
	return &models.SummarizedContent{SummaryJapanese: "テスト要約"}, nil
}

// stubSummaryRepoForWorker tracks calls to Create.
type stubSummaryRepoForWorker struct {
	repository.SummaryRepository
	createCalls int
}

func (m *stubSummaryRepoForWorker) Create(_ context.Context, _ *models.ArticleSummary) error {
	m.createCalls++
	return nil
}

func TestSummarizeQueueWorker_ProcessQueue_ContextCanceled(t *testing.T) {
	t.Run("should skip remaining jobs when context is canceled after fetching", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		jobs := []*models.SummarizeJob{
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

		jobs := []*models.SummarizeJob{
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
