package worker

import (
	"context"
	"errors"
	"log/slog"
	"io"
	"sync"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- stubs ---

type stubJobRepo struct {
	mu   sync.Mutex
	jobs []*domain.RagJob // jobs to return from AcquireNextJob (consumed FIFO)
	err  error
}

func (s *stubJobRepo) Enqueue(ctx context.Context, job *domain.RagJob) error { return nil }

func (s *stubJobRepo) AcquireNextJob(ctx context.Context) (*domain.RagJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	if len(s.jobs) == 0 {
		return nil, nil
	}
	job := s.jobs[0]
	s.jobs = s.jobs[1:]
	return job, nil
}

func (s *stubJobRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMessage *string) error {
	return nil
}

type stubIndexUsecase struct {
	mu          sync.Mutex
	capturedCtx context.Context
	returnErr   error
}

func (s *stubIndexUsecase) Upsert(ctx context.Context, articleID, title, url, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capturedCtx = ctx
	return s.returnErr
}

func (s *stubIndexUsecase) Delete(ctx context.Context, articleID string) error {
	return nil
}

func makeJob() *domain.RagJob {
	return &domain.RagJob{
		ID:      uuid.New(),
		JobType: "backfill_article",
		Payload: map[string]interface{}{
			"article_id": "art-1",
			"title":      "Test",
			"body":       "Body",
			"url":        "https://example.com",
		},
		Status: "processing",
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// --- tests ---

func TestProcessNextJob_ContextHasTimeout(t *testing.T) {
	uc := &stubIndexUsecase{}
	repo := &stubJobRepo{jobs: []*domain.RagJob{makeJob()}}

	w := NewJobWorker(repo, uc, testLogger())
	w.processNextJob()

	uc.mu.Lock()
	defer uc.mu.Unlock()

	assert.NotNil(t, uc.capturedCtx, "Upsert should have been called")
	deadline, ok := uc.capturedCtx.Deadline()
	assert.True(t, ok, "context passed to Upsert must have a deadline")
	assert.WithinDuration(t, time.Now().Add(jobTimeout), deadline, 5*time.Second)
}

func TestJobWorker_BacksOffOnConsecutiveFailures(t *testing.T) {
	repo := &stubJobRepo{
		jobs: []*domain.RagJob{makeJob(), makeJob(), makeJob()},
	}
	uc := &stubIndexUsecase{returnErr: errors.New("embedder unreachable")}

	w := NewJobWorker(repo, uc, testLogger())

	// First failure: backoff should be initialBackoff (1s)
	w.processNextJob()
	assert.Equal(t, initialBackoff, w.backoff)

	// Second failure: backoff doubles to 2s
	w.processNextJob()
	assert.Equal(t, 2*time.Second, w.backoff)

	// Third failure: backoff doubles to 4s
	w.processNextJob()
	assert.Equal(t, 4*time.Second, w.backoff)
}

func TestJobWorker_BackoffResetsOnSuccess(t *testing.T) {
	repo := &stubJobRepo{
		jobs: []*domain.RagJob{makeJob(), makeJob()},
	}
	uc := &stubIndexUsecase{returnErr: errors.New("fail")}

	w := NewJobWorker(repo, uc, testLogger())

	// Failure sets backoff
	w.processNextJob()
	assert.Equal(t, initialBackoff, w.backoff)

	// Now succeed
	uc.mu.Lock()
	uc.returnErr = nil
	uc.mu.Unlock()

	w.processNextJob()
	assert.Equal(t, time.Duration(0), w.backoff, "backoff should reset on success")
}

func TestJobWorker_BackoffCapsAtMax(t *testing.T) {
	w := NewJobWorker(nil, nil, testLogger())

	bo := time.Duration(0)
	for i := 0; i < 20; i++ {
		bo = w.nextBackoff(bo)
	}
	assert.Equal(t, maxBackoff, bo, "backoff must cap at maxBackoff")
	assert.LessOrEqual(t, bo, maxBackoff)
}
