package backfill

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubArticleSource struct {
	articles []Article
	err      error
}

func (s *stubArticleSource) ScanArticles(_ context.Context, fn func(Article) error) error {
	if s.err != nil {
		return s.err
	}
	for _, a := range s.articles {
		if err := fn(a); err != nil {
			return err
		}
	}
	return nil
}

func sourceArticle(id string) Article {
	return Article{
		ID:        id,
		Title:     "title " + id,
		Body:      "body of " + id,
		URL:       "https://example.test/" + id,
		CreatedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}
}

func testEnqueueConfig(target RebuildTarget) EnqueueConfig {
	cfg := DefaultEnqueueConfig()
	cfg.Target = target
	return cfg
}

func TestNewEnqueuer_FailsFastWithoutTarget(t *testing.T) {
	_, err := NewEnqueuer(&stubArticleSource{}, newStubVersionState(), newStubJobQueue(),
		testEnqueueConfig(RebuildTarget{ChunkerVersion: "v10"}), rebuildTestLogger())
	assert.Error(t, err, "an enqueue without a target embedder version must not run")

	_, err = NewEnqueuer(&stubArticleSource{}, newStubVersionState(), newStubJobQueue(),
		testEnqueueConfig(RebuildTarget{EmbedderVersion: "test-model/1024"}), rebuildTestLogger())
	assert.Error(t, err)
}

func TestEnqueuer_QueuesEveryStaleArticleWithAFullPayload(t *testing.T) {
	source := &stubArticleSource{articles: []Article{sourceArticle("a"), sourceArticle("b")}}
	queue := newStubJobQueue()

	enq, err := NewEnqueuer(source, newStubVersionState(), queue, testEnqueueConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	stats, err := enq.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.Scanned)
	assert.Equal(t, int64(2), stats.Enqueued)
	require.Len(t, queue.enqueued, 2)

	job := queue.enqueued[0]
	assert.Equal(t, rebuildJobType, job.JobType)
	assert.Equal(t, "new", job.Status)
	assert.Equal(t, "a", job.Payload["article_id"])
	assert.Equal(t, "title a", job.Payload["title"])
	assert.Equal(t, "body of a", job.Payload["body"])
	assert.Equal(t, "https://example.test/a", job.Payload["url"])
}

func TestEnqueuer_SkipsArticlesAlreadyAtTarget(t *testing.T) {
	source := &stubArticleSource{articles: []Article{sourceArticle("done"), sourceArticle("stale")}}
	state := newStubVersionState()
	state.set("done", VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	state.set("stale", VersionState{ChunkerVersion: "v9", EmbedderVersion: "embeddinggemma"})
	queue := newStubJobQueue()

	enq, err := NewEnqueuer(source, state, queue, testEnqueueConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	stats, err := enq.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Enqueued)
	assert.Equal(t, int64(1), stats.SkippedUpToDate)
	require.Len(t, queue.enqueued, 1)
	assert.Equal(t, "stale", queue.enqueued[0].Payload["article_id"])
}

func TestEnqueuer_SkipUpToDateOffQueuesEverything(t *testing.T) {
	source := &stubArticleSource{articles: []Article{sourceArticle("done")}}
	state := newStubVersionState()
	state.set("done", VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})

	cfg := testEnqueueConfig(testTarget())
	cfg.SkipUpToDate = false
	queue := newStubJobQueue()

	enq, err := NewEnqueuer(source, state, queue, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := enq.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Enqueued)
	assert.Zero(t, stats.SkippedUpToDate)
}

func TestEnqueuer_DryRunWritesNothing(t *testing.T) {
	source := &stubArticleSource{articles: []Article{sourceArticle("a"), sourceArticle("b")}}
	cfg := testEnqueueConfig(testTarget())
	cfg.DryRun = true
	queue := newStubJobQueue()

	enq, err := NewEnqueuer(source, newStubVersionState(), queue, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := enq.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.Enqueued, "a dry run still reports what it would queue")
	assert.Empty(t, queue.enqueued)
}

func TestEnqueuer_SkipsEmptyBodies(t *testing.T) {
	empty := sourceArticle("empty")
	empty.Body = ""
	source := &stubArticleSource{articles: []Article{empty, sourceArticle("ok")}}
	queue := newStubJobQueue()

	enq, err := NewEnqueuer(source, newStubVersionState(), queue, testEnqueueConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	stats, err := enq.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.SkippedEmpty)
	assert.Equal(t, int64(1), stats.Enqueued)
	require.Len(t, queue.enqueued, 1)
	assert.Equal(t, "ok", queue.enqueued[0].Payload["article_id"])
}

func TestEnqueuer_SurfacesSourceErrors(t *testing.T) {
	source := &stubArticleSource{err: errors.New("source database unreachable")}

	enq, err := NewEnqueuer(source, newStubVersionState(), newStubJobQueue(), testEnqueueConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	_, err = enq.Run(context.Background())
	assert.ErrorContains(t, err, "source database unreachable")
}

func TestEnqueuer_SurfacesStateReaderErrors(t *testing.T) {
	state := newStubVersionState()
	state.err = errors.New("rag-db unreachable")

	enq, err := NewEnqueuer(&stubArticleSource{}, state, newStubJobQueue(), testEnqueueConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	_, err = enq.Run(context.Background())
	assert.ErrorContains(t, err, "rag-db unreachable")
}
