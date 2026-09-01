package backfill

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ----------------------------------------------------------------

type stubJobQueue struct {
	mu        sync.Mutex
	pending   []*domain.RagJob
	statuses  map[uuid.UUID]string
	errors    map[uuid.UUID]string
	enqueued  []*domain.RagJob
	acquerror error
}

func newStubJobQueue(jobs ...*domain.RagJob) *stubJobQueue {
	return &stubJobQueue{
		pending:  jobs,
		statuses: map[uuid.UUID]string{},
		errors:   map[uuid.UUID]string{},
	}
}

func (s *stubJobQueue) Enqueue(_ context.Context, job *domain.RagJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enqueued = append(s.enqueued, job)
	return nil
}

func (s *stubJobQueue) AcquireNextJob(context.Context) (*domain.RagJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acquerror != nil {
		return nil, s.acquerror
	}
	if len(s.pending) == 0 {
		return nil, nil
	}
	job := s.pending[0]
	s.pending = s.pending[1:]
	return job, nil
}

func (s *stubJobQueue) UpdateStatus(_ context.Context, id uuid.UUID, status string, errorMessage *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[id] = status
	if errorMessage != nil {
		s.errors[id] = *errorMessage
	}
	return nil
}

func (s *stubJobQueue) statusOf(id uuid.UUID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statuses[id]
}

func (s *stubJobQueue) errorOf(id uuid.UUID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errors[id]
}

type stubIndexer struct {
	mu           sync.Mutex
	calls        []string
	err          error
	delay        time.Duration
	inFlight     atomic.Int64
	maxInFlight  atomic.Int64
	onUpsertDone func(articleID string)
}

func (s *stubIndexer) Upsert(ctx context.Context, articleID, title, url, body string) error {
	cur := s.inFlight.Add(1)
	for {
		max := s.maxInFlight.Load()
		if cur <= max || s.maxInFlight.CompareAndSwap(max, cur) {
			break
		}
	}
	defer s.inFlight.Add(-1)

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	s.mu.Lock()
	s.calls = append(s.calls, articleID)
	s.mu.Unlock()

	if s.onUpsertDone != nil {
		s.onUpsertDone(articleID)
	}
	return s.err
}

func (s *stubIndexer) Delete(context.Context, string) error { return nil }

func (s *stubIndexer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

type stubVersionState struct {
	mu    sync.Mutex
	state map[string]VersionState
	err   error
}

func newStubVersionState() *stubVersionState {
	return &stubVersionState{state: map[string]VersionState{}}
}

func (s *stubVersionState) CurrentState(_ context.Context, articleID string) (VersionState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return VersionState{}, false, s.err
	}
	st, ok := s.state[articleID]
	return st, ok, nil
}

func (s *stubVersionState) ArticleIDsAt(_ context.Context, target RebuildTarget) (map[string]struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	out := map[string]struct{}{}
	for id, st := range s.state {
		if st.ChunkerVersion == target.ChunkerVersion && st.EmbedderVersion == target.EmbedderVersion {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func (s *stubVersionState) set(articleID string, st VersionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[articleID] = st
}

func rebuildTestLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func testTarget() RebuildTarget {
	return RebuildTarget{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"}
}

func rebuildJob(articleID string) *domain.RagJob {
	return &domain.RagJob{
		ID:      uuid.New(),
		JobType: rebuildJobType,
		Payload: map[string]interface{}{
			"article_id": articleID,
			"title":      "title " + articleID,
			"url":        "https://example.test/" + articleID,
			"body":       "body of " + articleID,
		},
		Status: "processing",
	}
}

func testRebuildConfig(target RebuildTarget) RebuildConfig {
	cfg := DefaultRebuildConfig()
	cfg.Target = target
	cfg.Workers = 3
	cfg.IdleTimeout = 150 * time.Millisecond
	cfg.ProgressInterval = time.Hour // progress logging is not under test
	cfg.JobTimeout = 5 * time.Second
	return cfg
}

// --- configuration --------------------------------------------------------

func TestNewRebuildEngine_FailsFastOnIncompleteConfig(t *testing.T) {
	jobs := newStubJobQueue()
	state := newStubVersionState()
	indexers := []usecase.IndexArticleUsecase{&stubIndexer{}}

	tests := []struct {
		name     string
		cfg      RebuildConfig
		indexers []usecase.IndexArticleUsecase
	}{
		{
			name: "missing embedder version",
			cfg: func() RebuildConfig {
				c := testRebuildConfig(RebuildTarget{ChunkerVersion: "v10"})
				return c
			}(),
			indexers: indexers,
		},
		{
			name:     "missing chunker version",
			cfg:      testRebuildConfig(RebuildTarget{EmbedderVersion: "test-model/1024"}),
			indexers: indexers,
		},
		{
			name: "no workers",
			cfg: func() RebuildConfig {
				c := testRebuildConfig(testTarget())
				c.Workers = 0
				return c
			}(),
			indexers: indexers,
		},
		{
			name:     "no indexers",
			cfg:      testRebuildConfig(testTarget()),
			indexers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewRebuildEngine(jobs, tt.indexers, state, tt.cfg, rebuildTestLogger())
			assert.Error(t, err, "incomplete rebuild config must fail at construction")
			assert.Nil(t, engine)
		})
	}
}

func TestDefaultRebuildConfig_LeavesTargetUnset(t *testing.T) {
	cfg := DefaultRebuildConfig()
	assert.Empty(t, cfg.Target.EmbedderVersion, "the embedding model is chosen by evaluation, never defaulted here")
	assert.Empty(t, cfg.Target.ChunkerVersion)
	assert.Positive(t, cfg.Workers)
}

// --- draining -------------------------------------------------------------

func TestRebuildEngine_DrainsQueueWithBoundedParallelism(t *testing.T) {
	var jobs []*domain.RagJob
	for i := 0; i < 6; i++ {
		jobs = append(jobs, rebuildJob(uuid.NewString()))
	}
	queue := newStubJobQueue(jobs...)
	state := newStubVersionState()
	indexer := &stubIndexer{delay: 20 * time.Millisecond}
	indexer.onUpsertDone = func(articleID string) {
		state.set(articleID, VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	}

	cfg := testRebuildConfig(testTarget())
	cfg.TotalJobs = int64(len(jobs))
	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{indexer}, state, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(6), stats.Rebuilt)
	assert.Zero(t, stats.Failed)
	assert.Equal(t, 6, indexer.callCount())
	assert.GreaterOrEqual(t, indexer.maxInFlight.Load(), int64(2), "workers must run in parallel")
	assert.LessOrEqual(t, indexer.maxInFlight.Load(), int64(cfg.Workers), "parallelism must stay bounded")

	for _, j := range jobs {
		assert.Equal(t, "completed", queue.statusOf(j.ID))
	}
}

func TestRebuildEngine_SkipsDocumentsAlreadyAtTarget(t *testing.T) {
	fresh := rebuildJob("already-current")
	stale := rebuildJob("stale")
	queue := newStubJobQueue(fresh, stale)

	state := newStubVersionState()
	state.set("already-current", VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	state.set("stale", VersionState{ChunkerVersion: "v9", EmbedderVersion: "embeddinggemma"})

	indexer := &stubIndexer{}
	indexer.onUpsertDone = func(articleID string) {
		state.set(articleID, VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	}

	cfg := testRebuildConfig(testTarget())
	cfg.Workers = 1
	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{indexer}, state, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Skipped)
	assert.Equal(t, int64(1), stats.Rebuilt)
	assert.Equal(t, []string{"stale"}, indexer.calls, "an up-to-date document must not be re-embedded")
	assert.Equal(t, "completed", queue.statusOf(fresh.ID))
}

func TestRebuildEngine_ResumeIsIdempotent(t *testing.T) {
	// A second pass over the same articles must do no embedding work.
	state := newStubVersionState()
	state.set("a", VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	state.set("b", VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})

	queue := newStubJobQueue(rebuildJob("a"), rebuildJob("b"))
	indexer := &stubIndexer{}

	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{indexer}, state, testRebuildConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.Skipped)
	assert.Zero(t, stats.Rebuilt)
	assert.Zero(t, indexer.callCount())
}

// --- loud failures --------------------------------------------------------

func TestRebuildEngine_FailsLoudlyWhenUpsertDoesNotReachTarget(t *testing.T) {
	// Upsert reports success but the document version never moves: the classic
	// silent no-op that leaves a rebuild "complete" over a stale corpus.
	job := rebuildJob("never-moves")
	queue := newStubJobQueue(job)
	state := newStubVersionState()
	state.set("never-moves", VersionState{ChunkerVersion: "v9", EmbedderVersion: "embeddinggemma"})

	cfg := testRebuildConfig(testTarget())
	cfg.Workers = 1
	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{&stubIndexer{}}, state, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Failed)
	assert.Zero(t, stats.Rebuilt)
	assert.Equal(t, "failed", queue.statusOf(job.ID))
	assert.Contains(t, queue.errorOf(job.ID), "test-model/1024")
}

func TestRebuildEngine_RejectsJobWithoutArticleID(t *testing.T) {
	job := rebuildJob("x")
	delete(job.Payload, "article_id")
	queue := newStubJobQueue(job)

	cfg := testRebuildConfig(testTarget())
	cfg.Workers = 1
	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{&stubIndexer{}}, newStubVersionState(), cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats.Failed)
	assert.Equal(t, "failed", queue.statusOf(job.ID))
}

func TestRebuildEngine_AbortsAfterConsecutiveFailures(t *testing.T) {
	var jobs []*domain.RagJob
	for i := 0; i < 200; i++ {
		jobs = append(jobs, rebuildJob(uuid.NewString()))
	}
	queue := newStubJobQueue(jobs...)

	cfg := testRebuildConfig(testTarget())
	cfg.Workers = 2
	cfg.MaxConsecutiveFailures = 4
	indexer := &stubIndexer{err: errors.New("embedder unreachable")}

	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{indexer}, newStubVersionState(), cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.Error(t, err, "a rebuild must stop rather than burn the whole queue against a dead embedder")
	assert.Contains(t, err.Error(), "consecutive")
	assert.Less(t, stats.Failed, int64(len(jobs)))
}

func TestRebuildEngine_StopsOnCanceledContext(t *testing.T) {
	var jobs []*domain.RagJob
	for i := 0; i < 50; i++ {
		jobs = append(jobs, rebuildJob(uuid.NewString()))
	}
	queue := newStubJobQueue(jobs...)
	state := newStubVersionState()
	indexer := &stubIndexer{delay: 30 * time.Millisecond}
	indexer.onUpsertDone = func(articleID string) {
		state.set(articleID, VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	}

	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{indexer}, state, testRebuildConfig(testTarget()), rebuildTestLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	stats, err := engine.Run(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, stats.Rebuilt, int64(len(jobs)))
}

func TestRebuildEngine_DistributesWorkAcrossEmbedderReplicas(t *testing.T) {
	var jobs []*domain.RagJob
	for i := 0; i < 8; i++ {
		jobs = append(jobs, rebuildJob(uuid.NewString()))
	}
	queue := newStubJobQueue(jobs...)
	state := newStubVersionState()

	first := &stubIndexer{delay: 10 * time.Millisecond}
	second := &stubIndexer{delay: 10 * time.Millisecond}
	mark := func(articleID string) {
		state.set(articleID, VersionState{ChunkerVersion: "v10", EmbedderVersion: "test-model/1024"})
	}
	first.onUpsertDone = mark
	second.onUpsertDone = mark

	cfg := testRebuildConfig(testTarget())
	cfg.Workers = 2
	engine, err := NewRebuildEngine(queue, []usecase.IndexArticleUsecase{first, second}, state, cfg, rebuildTestLogger())
	require.NoError(t, err)

	stats, err := engine.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int64(8), stats.Rebuilt)
	assert.Positive(t, first.callCount())
	assert.Positive(t, second.callCount())
}
