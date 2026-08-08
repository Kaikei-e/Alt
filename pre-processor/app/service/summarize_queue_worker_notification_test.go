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

// stubJobRepoCompletion records which completion path the worker took, so the
// test can tell a job that merely finished from one that also enqueued its
// notification.
type stubJobRepoCompletion struct {
	repository.SummarizeJobRepository
	jobs []*domain.SummarizeJob

	mu          sync.Mutex
	completions []completionCall
	updates     []domain.SummarizeJobStatus
}

func (m *stubJobRepoCompletion) DequeueJobs(_ context.Context, _ int) ([]*domain.SummarizeJob, error) {
	return m.jobs, nil
}

func (m *stubJobRepoCompletion) RecoverStuckJobs(_ context.Context) (int64, error) { return 0, nil }

func (m *stubJobRepoCompletion) UpdateJobStatus(_ context.Context, _ string, status domain.SummarizeJobStatus, _ string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, status)
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

// TestSummarizeQueueWorker_CompletesThroughTheOutbox is the wiring half of the
// producer: a summary that reaches the user has to be completed through the
// transactional path, or the outbox row is never written and the feature is
// dead code with passing unit tests.
func TestSummarizeQueueWorker_CompletesThroughTheOutbox(t *testing.T) {
	jobID := uuid.New()
	jobRepo := &stubJobRepoCompletion{
		jobs: []*domain.SummarizeJob{{JobID: jobID, ArticleID: "article-1", MaxRetries: 3}},
	}

	worker := NewSummarizeQueueWorker(
		jobRepo,
		&stubArticleRepoForWorker{},
		&stubAPIRepoForWorker{},
		&stubSummaryRepoForWorker{},
		testLogger(),
		10,
	)

	require.NoError(t, worker.ProcessQueue(context.Background()))

	require.Len(t, jobRepo.completions, 1)
	assert.Equal(t, jobID.String(), jobRepo.completions[0].jobID)
	assert.Equal(t, "テスト要約", jobRepo.completions[0].summary)
	assert.Equal(t, "user-1", jobRepo.completions[0].userID,
		"the notification's recipient comes from the article's owner")
	assert.Equal(t, "article-1", jobRepo.completions[0].articleID)

	assert.NotContains(t, jobRepo.updates, domain.SummarizeJobStatusCompleted,
		"a successful summary must not be completed through the non-transactional path")
}
