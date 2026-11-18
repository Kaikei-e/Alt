package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
	"pre-processor/repository"
)

type completionCall struct {
	jobID     string
	summary   string
	userID    string
	articleID string
}

type statusUpdateCall struct {
	jobID    string
	status   domain.SummarizeJobStatus
	summary  string
	errorMsg string
}

type stubJobRepoCompletion struct {
	repository.SummarizeJobRepository
	jobs []*domain.SummarizeJob

	mu          sync.Mutex
	completions []completionCall
	updates     []statusUpdateCall
}

func (m *stubJobRepoCompletion) DequeueJobs(_ context.Context, limit int) ([]*domain.SummarizeJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.jobs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > len(m.jobs) {
		limit = len(m.jobs)
	}
	out := m.jobs[:limit]
	m.jobs = m.jobs[limit:]
	return out, nil
}

func (m *stubJobRepoCompletion) RecoverStuckJobs(_ context.Context) (int64, error) { return 0, nil }

func (m *stubJobRepoCompletion) UpdateJobStatus(_ context.Context, jobID string, status domain.SummarizeJobStatus, summary string, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, statusUpdateCall{
		jobID:    jobID,
		status:   status,
		summary:  summary,
		errorMsg: errorMsg,
	})
	return nil
}

func (m *stubJobRepoCompletion) CompleteJobWithNotification(_ context.Context, jobID, summary, userID, articleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completions = append(m.completions, completionCall{
		jobID:     jobID,
		summary:   summary,
		userID:    userID,
		articleID: articleID,
	})
	return nil
}

type stubArticleRepoNoUser struct {
	repository.ArticleRepository
}

func (m *stubArticleRepoNoUser) FindByID(_ context.Context, articleID string) (*domain.Article, error) {
	return &domain.Article{
		ID:      articleID,
		UserID:  "",
		Title:   "Ownerless Article",
		Content: "Test content for summarization",
	}, nil
}

type stubCompletionAPIRepo struct {
	repository.ExternalAPIRepository
}

func (m *stubCompletionAPIRepo) SummarizeArticle(_ context.Context, _ *domain.Article, _ string) (*domain.SummarizedContent, error) {
	return &domain.SummarizedContent{SummaryJapanese: "Generated summary"}, nil
}

func TestSummarizeQueueWorker_UserOriginInsertsNotification(t *testing.T) {
	jobID := uuid.New()
	jobRepo := &stubJobRepoCompletion{
		jobs: []*domain.SummarizeJob{{
			JobID:      jobID,
			ArticleID:  "article-1",
			Origin:     domain.SummarizeOriginUser,
			MaxRetries: 3,
		}},
	}

	worker := NewSummarizeQueueWorker(
		jobRepo,
		&stubArticleRepoForWorker{},
		&stubCompletionAPIRepo{},
		&stubSummaryRepoForWorker{},
		testLogger(),
	)

	require.NoError(t, worker.ProcessQueue(context.Background(), noRefill))

	require.Len(t, jobRepo.completions, 1)
	assert.Equal(t, jobID.String(), jobRepo.completions[0].jobID)
	assert.Equal(t, "Generated summary", jobRepo.completions[0].summary)
	assert.Equal(t, "user-1", jobRepo.completions[0].userID)
	assert.Equal(t, "article-1", jobRepo.completions[0].articleID)
	assert.Empty(t, jobRepo.updates)
}

func TestSummarizeQueueWorker_SystemOriginSkipsNotification(t *testing.T) {
	jobID := uuid.New()
	jobRepo := &stubJobRepoCompletion{
		jobs: []*domain.SummarizeJob{{
			JobID:      jobID,
			ArticleID:  "article-1",
			Origin:     domain.SummarizeOriginSystem,
			MaxRetries: 3,
		}},
	}

	worker := NewSummarizeQueueWorker(
		jobRepo,
		&stubArticleRepoForWorker{},
		&stubCompletionAPIRepo{},
		&stubSummaryRepoForWorker{},
		testLogger(),
	)

	require.NoError(t, worker.ProcessQueue(context.Background(), noRefill))

	assert.Empty(t, jobRepo.completions, "system-origin jobs must not enqueue a summary_ready notification")
	require.Len(t, jobRepo.updates, 1)
	assert.Equal(t, statusUpdateCall{
		jobID:    jobID.String(),
		status:   domain.SummarizeJobStatusCompleted,
		summary:  "Generated summary",
		errorMsg: "",
	}, jobRepo.updates[0])
}

func TestSummarizeQueueWorker_UserOriginFailsExplicitlyWithoutUserID(t *testing.T) {
	jobID := uuid.New()
	jobRepo := &stubJobRepoCompletion{
		jobs: []*domain.SummarizeJob{{
			JobID:      jobID,
			ArticleID:  "article-no-user",
			Origin:     domain.SummarizeOriginUser,
			MaxRetries: 3,
		}},
	}

	worker := NewSummarizeQueueWorker(
		jobRepo,
		&stubArticleRepoNoUser{},
		&stubAPIRepoForWorker{},
		&stubSummaryRepoForWorker{},
		testLogger(),
	)

	err := worker.processJob(context.Background(), jobRepo.jobs[0])
	require.Error(t, err)
	assert.Empty(t, jobRepo.completions, "must not invent a user ID or enqueue an unaddressed notification")
	assert.Empty(t, jobRepo.updates, "must not silently downgrade user-origin completion to non-notifying status update")
}

func TestSummarizeQueueWorker_InvalidOriginFailsExplicitly(t *testing.T) {
	jobID := uuid.New()
	jobRepo := &stubJobRepoCompletion{
		jobs: []*domain.SummarizeJob{{
			JobID:      jobID,
			ArticleID:  "article-1",
			Origin:     "",
			MaxRetries: 3,
		}},
	}

	worker := NewSummarizeQueueWorker(
		jobRepo,
		&stubArticleRepoForWorker{},
		&stubAPIRepoForWorker{},
		&stubSummaryRepoForWorker{},
		testLogger(),
	)

	err := worker.processJob(context.Background(), jobRepo.jobs[0])
	require.ErrorIs(t, err, domain.ErrInvalidSummarizeOrigin)
	assert.Empty(t, jobRepo.completions)
	assert.Empty(t, jobRepo.updates)
}
