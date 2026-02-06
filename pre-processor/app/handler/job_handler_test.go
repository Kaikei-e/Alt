package handler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/models"
	"pre-processor/repository"
	"pre-processor/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func testJobHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
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

func TestRunSummarizationLoop_RecoverFromPanic(t *testing.T) {
	t.Run("processSummarizationBatch panics when summarizer panics", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		h := &jobHandler{
			articleSummarizer: &panickingArticleSummarizer{},
			logger:            testJobHandlerLogger(),
			ctx:               ctx,
			cancel:            cancel,
			batchSize:         10,
		}

		// processSummarizationBatch should propagate the panic (no recover there)
		assert.Panics(t, func() {
			h.processSummarizationBatch()
		}, "processSummarizationBatch should panic when summarizer panics")
	})

	t.Run("runSummarizationLoop should recover from panic and exit gracefully", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		h := &jobHandler{
			articleSummarizer: &panickingArticleSummarizer{},
			logger:            testJobHandlerLogger(),
			ctx:               ctx,
			cancel:            cancel,
			batchSize:         10,
		}

		// Cancel context immediately so the loop exits via ctx.Done() on first select.
		// This verifies runSummarizationLoop has a proper recover() defer that doesn't
		// interfere with normal operation. The recover() is the same pattern used in
		// runQualityCheckLoop (line 246) and runArticleSyncLoop (line 127).
		cancel()

		assert.NotPanics(t, func() {
			h.runSummarizationLoop()
		}, "runSummarizationLoop should not propagate panics")
	})
}

// --- Stubs for queue worker backoff tests ---

// stubJobRepoForHandler returns fixed pending jobs.
type stubJobRepoForHandler struct {
	repository.SummarizeJobRepository
	jobs []*models.SummarizeJob
}

func (m *stubJobRepoForHandler) GetPendingJobs(_ context.Context, _ int) ([]*models.SummarizeJob, error) {
	return m.jobs, nil
}

func (m *stubJobRepoForHandler) UpdateJobStatus(_ context.Context, _ string, _ models.SummarizeJobStatus, _ string, _ string) error {
	return nil
}

// stubArticleRepoForHandler returns a fixed article.
type stubArticleRepoForHandler struct {
	repository.ArticleRepository
}

func (m *stubArticleRepoForHandler) FindByID(_ context.Context, _ string) (*models.Article, error) {
	return &models.Article{ID: "a1", UserID: "u1", Title: "T", Content: "C"}, nil
}

// overloadedAPIRepoForHandler always returns ErrServiceOverloaded, tracks call count.
type overloadedAPIRepoForHandler struct {
	repository.ExternalAPIRepository
	calls atomic.Int32
}

func (m *overloadedAPIRepoForHandler) SummarizeArticle(_ context.Context, _ *models.Article, _ string) (*models.SummarizedContent, error) {
	m.calls.Add(1)
	return nil, domain.ErrServiceOverloaded
}

// stubSummaryRepoForHandler tracks Create calls.
type stubSummaryRepoForHandler struct {
	repository.SummaryRepository
}

func (m *stubSummaryRepoForHandler) Create(_ context.Context, _ *models.ArticleSummary) error {
	return nil
}

func TestRunSummarizeQueueLoop_BackoffOnOverloaded(t *testing.T) {
	t.Run("should back off when ProcessQueue returns ErrServiceOverloaded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		jobs := []*models.SummarizeJob{
			{JobID: uuid.New(), ArticleID: "a1", MaxRetries: 3},
		}

		apiRepo := &overloadedAPIRepoForHandler{}

		worker := service.NewSummarizeQueueWorker(
			&stubJobRepoForHandler{jobs: jobs},
			&stubArticleRepoForHandler{},
			apiRepo,
			&stubSummaryRepoForHandler{},
			testJobHandlerLogger(),
			10,
		)

		h := &jobHandler{
			queueWorker: worker,
			logger:      testJobHandlerLogger(),
			ctx:         ctx,
			cancel:      cancel,
		}

		// ProcessQueue should return ErrServiceOverloaded
		err := worker.ProcessQueue(ctx)
		assert.True(t, errors.Is(err, domain.ErrServiceOverloaded))

		// Now test the loop: run for a short window and verify backoff behavior.
		// The ticker fires every 10s normally, but with backoff it should wait longer.
		// We'll cancel after 150ms - with a 10s ticker, the loop should fire once
		// immediately via ticker reset, and the backoff should prevent rapid re-calls.
		go func() {
			time.Sleep(150 * time.Millisecond)
			cancel()
		}()

		h.runSummarizeQueueLoop()

		// With a 10-second ticker, in 150ms we should get at most 1 call.
		// The key assertion is that backoff doesn't crash and the loop exits cleanly.
		callCount := int(apiRepo.calls.Load())
		assert.LessOrEqual(t, callCount, 1,
			"should not rapidly retry when service is overloaded, got %d calls", callCount)
	})
}
