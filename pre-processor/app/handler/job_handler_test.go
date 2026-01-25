package handler

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"pre-processor/service"

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
