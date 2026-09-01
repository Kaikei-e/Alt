package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"
)

const (
	// rebuildJobType is the rag_jobs job type the enqueuer writes and the
	// rebuild engine drains. It matches the type the always-on JobWorker
	// understands, so a job left behind by a rebuild is still processed.
	rebuildJobType = "backfill_article"

	// rebuildPollInterval is how long a worker waits before asking for another
	// job once the queue came back empty.
	rebuildPollInterval = 200 * time.Millisecond

	// statusUpdateTimeout bounds the detached write that records a job's
	// outcome.
	statusUpdateTimeout = 10 * time.Second

	// defaultRebuildWorkers keeps the embedder busy without thrashing it.
	// knowledge-embedder-local runs with OLLAMA_NUM_PARALLEL=2, so two calls
	// are served concurrently and further calls queue inside Ollama, adding
	// latency without adding throughput. Three workers keep both slots fed
	// while one worker is chunking or writing to the database.
	defaultRebuildWorkers = 3
)

// VersionState is the derivation state recorded on a document's current
// version. It is the only thing that distinguishes a rebuilt document from one
// the indexer silently declined to touch.
type VersionState struct {
	ChunkerVersion  string
	EmbedderVersion string
}

// RebuildTarget names the derivation the corpus is being rebuilt onto. Both
// halves are supplied by the caller: the embedding model is chosen by
// evaluation, so nothing here may carry a default model name.
type RebuildTarget struct {
	ChunkerVersion  string
	EmbedderVersion string
}

func (t RebuildTarget) matches(s VersionState) bool {
	return s.ChunkerVersion == t.ChunkerVersion && s.EmbedderVersion == t.EmbedderVersion
}

func (t RebuildTarget) validate() error {
	if t.ChunkerVersion == "" {
		return fmt.Errorf("rebuild target: chunker version is required")
	}
	if t.EmbedderVersion == "" {
		return fmt.Errorf("rebuild target: embedder version is required")
	}
	return nil
}

// VersionStateReader reads the derivation state of documents' current versions
// from the index.
type VersionStateReader interface {
	// CurrentState returns the state recorded on articleID's current version.
	// The second result is false when the article has no indexed version yet.
	CurrentState(ctx context.Context, articleID string) (VersionState, bool, error)

	// ArticleIDsAt returns the set of article IDs whose current version already
	// matches target.
	ArticleIDsAt(ctx context.Context, target RebuildTarget) (map[string]struct{}, error)
}

// RebuildConfig configures a one-shot full rebuild.
type RebuildConfig struct {
	// Target is the chunker/embedder pair every document must end up at.
	Target RebuildTarget
	// Workers is the number of documents embedded concurrently.
	Workers int
	// JobTimeout bounds one document: chunking, embedding and the write.
	JobTimeout time.Duration
	// ProgressInterval is how often progress and ETA are logged.
	ProgressInterval time.Duration
	// IdleTimeout is how long the queue must stay empty before the run is
	// considered drained.
	IdleTimeout time.Duration
	// MaxConsecutiveFailures aborts the run once this many jobs fail in a row,
	// rather than burning the whole queue against a dead embedder.
	MaxConsecutiveFailures int
	// TotalJobs is the queue depth at start. Zero means unknown, which only
	// costs the ETA in the progress log.
	TotalJobs int64
}

// DefaultRebuildConfig returns the operational defaults. Target is left unset
// on purpose: a rebuild that guesses its own target is how a corpus ends up in
// a mixed state nobody can query for.
func DefaultRebuildConfig() RebuildConfig {
	return RebuildConfig{
		Workers:                defaultRebuildWorkers,
		JobTimeout:             5 * time.Minute,
		ProgressInterval:       15 * time.Second,
		IdleTimeout:            30 * time.Second,
		MaxConsecutiveFailures: 20,
	}
}

// RebuildStats is the outcome of a rebuild run.
type RebuildStats struct {
	Rebuilt int64
	Skipped int64
	Failed  int64
}

type rebuildOutcome int

const (
	outcomeRebuilt rebuildOutcome = iota
	outcomeSkipped
)

// RebuildEngine drains rag_jobs with a bounded pool of workers, re-indexing
// every queued article until its current version reaches the target.
type RebuildEngine struct {
	jobs     domain.RagJobRepository
	indexers []usecase.IndexArticleUsecase
	state    VersionStateReader
	cfg      RebuildConfig
	logger   *slog.Logger

	rebuilt atomic.Int64
	skipped atomic.Int64
	failed  atomic.Int64

	consecutiveFailures atomic.Int64
	abortFlag           atomic.Bool
	abortOnce           sync.Once
	abortErr            error

	lastActivity atomic.Int64 // unix nanoseconds
}

// NewRebuildEngine validates the wiring up front. Every dependency is
// mandatory: a rebuild that runs with a missing target or a missing indexer
// would report success over an untouched corpus.
func NewRebuildEngine(
	jobs domain.RagJobRepository,
	indexers []usecase.IndexArticleUsecase,
	state VersionStateReader,
	cfg RebuildConfig,
	logger *slog.Logger,
) (*RebuildEngine, error) {
	if jobs == nil {
		return nil, fmt.Errorf("rebuild engine: job repository is required")
	}
	if len(indexers) == 0 {
		return nil, fmt.Errorf("rebuild engine: at least one index usecase is required")
	}
	if state == nil {
		return nil, fmt.Errorf("rebuild engine: version state reader is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("rebuild engine: logger is required")
	}
	if err := cfg.Target.validate(); err != nil {
		return nil, err
	}
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("rebuild engine: workers must be positive, got %d", cfg.Workers)
	}
	if cfg.JobTimeout <= 0 {
		return nil, fmt.Errorf("rebuild engine: job timeout must be positive, got %s", cfg.JobTimeout)
	}
	if cfg.IdleTimeout <= 0 {
		return nil, fmt.Errorf("rebuild engine: idle timeout must be positive, got %s", cfg.IdleTimeout)
	}

	return &RebuildEngine{
		jobs:     jobs,
		indexers: indexers,
		state:    state,
		cfg:      cfg,
		logger:   logger,
	}, nil
}

// Run drains the queue and returns once it has stayed empty for IdleTimeout,
// the context ends, or too many jobs fail in a row.
func (e *RebuildEngine) Run(ctx context.Context) (RebuildStats, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	start := time.Now()
	e.markActivity()

	e.logger.Info("rebuild_started",
		slog.Int("workers", e.cfg.Workers),
		slog.Int("embedder_replicas", len(e.indexers)),
		slog.String("target_chunker_version", e.cfg.Target.ChunkerVersion),
		slog.String("target_embedder_version", e.cfg.Target.EmbedderVersion),
		slog.Int64("total_jobs", e.cfg.TotalJobs),
		slog.Duration("job_timeout", e.cfg.JobTimeout),
	)

	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		e.reportProgress(runCtx, start)
	}()

	var wg sync.WaitGroup
	for i := 0; i < e.cfg.Workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			e.work(runCtx, worker)
		}(i)
	}
	wg.Wait()

	cancel()
	<-progressDone

	stats := e.snapshot()
	err := e.abortErr
	if err == nil {
		err = ctx.Err()
	}

	e.logger.Info("rebuild_finished",
		slog.Int64("rebuilt", stats.Rebuilt),
		slog.Int64("skipped", stats.Skipped),
		slog.Int64("failed", stats.Failed),
		slog.Duration("elapsed", time.Since(start)),
		slog.Bool("aborted", e.abortFlag.Load()),
	)

	return stats, err
}

// work pulls jobs until the queue drains, the run aborts, or ctx ends. Each
// worker is pinned to one embedder replica so a multi-replica run spreads
// evenly without a shared counter.
func (e *RebuildEngine) work(ctx context.Context, worker int) {
	indexer := e.indexers[worker%len(e.indexers)]

	for {
		if ctx.Err() != nil || e.abortFlag.Load() {
			return
		}

		job, err := e.jobs.AcquireNextJob(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Error("rebuild_acquire_failed", slog.String("error", err.Error()))
			e.recordFailure(fmt.Errorf("acquire next job: %w", err))
			if !sleepCtx(ctx, rebuildPollInterval) {
				return
			}
			continue
		}

		if job == nil {
			if time.Since(e.lastActivityAt()) > e.cfg.IdleTimeout {
				return
			}
			if !sleepCtx(ctx, rebuildPollInterval) {
				return
			}
			continue
		}

		e.markActivity()
		e.processJob(ctx, job, indexer)
	}
}

func (e *RebuildEngine) processJob(ctx context.Context, job *domain.RagJob, indexer usecase.IndexArticleUsecase) {
	jobCtx, cancel := context.WithTimeout(ctx, e.cfg.JobTimeout)
	defer cancel()

	outcome, err := e.rebuildArticle(jobCtx, job, indexer)

	status := "completed"
	var errMsg *string
	if err != nil {
		status = "failed"
		msg := err.Error()
		errMsg = &msg
		e.failed.Add(1)
		e.logger.Error("rebuild_job_failed",
			slog.String("job_id", job.ID.String()),
			slog.String("error", msg),
		)
		e.recordFailure(err)
	} else {
		e.consecutiveFailures.Store(0)
		if outcome == outcomeSkipped {
			e.skipped.Add(1)
		} else {
			e.rebuilt.Add(1)
		}
	}

	// The status write uses a fresh context detached from the job's: a job that
	// ran close to JobTimeout would otherwise fail to record its own outcome
	// and sit at 'processing' until the stale-lease reclaim picks it up.
	updateCtx, updateCancel := context.WithTimeout(context.Background(), statusUpdateTimeout)
	defer updateCancel()
	if updateErr := e.jobs.UpdateStatus(updateCtx, job.ID, status, errMsg); updateErr != nil {
		e.logger.Error("rebuild_status_update_failed",
			slog.String("job_id", job.ID.String()),
			slog.String("error", updateErr.Error()),
		)
	}
}

// rebuildArticle re-indexes one article and proves it landed.
//
// The read before the work is what makes a rerun cheap: an article already at
// the target costs one indexed lookup instead of an embedding call. The read
// after the work is what makes a silent no-op impossible: IndexArticleUsecase
// returns nil both when it rebuilt the document and when it decided nothing
// changed, and only the recorded version tells those apart.
func (e *RebuildEngine) rebuildArticle(ctx context.Context, job *domain.RagJob, indexer usecase.IndexArticleUsecase) (rebuildOutcome, error) {
	articleID, err := payloadString(job.Payload, "article_id")
	if err != nil {
		return outcomeRebuilt, err
	}
	title, err := payloadString(job.Payload, "title")
	if err != nil {
		return outcomeRebuilt, err
	}
	body, err := payloadString(job.Payload, "body")
	if err != nil {
		return outcomeRebuilt, err
	}
	url, _ := job.Payload["url"].(string)

	before, found, err := e.state.CurrentState(ctx, articleID)
	if err != nil {
		return outcomeRebuilt, fmt.Errorf("read current state of %s: %w", articleID, err)
	}
	if found && e.cfg.Target.matches(before) {
		return outcomeSkipped, nil
	}

	if err := indexer.Upsert(ctx, articleID, title, url, body); err != nil {
		return outcomeRebuilt, fmt.Errorf("upsert %s: %w", articleID, err)
	}

	after, found, err := e.state.CurrentState(ctx, articleID)
	if err != nil {
		return outcomeRebuilt, fmt.Errorf("verify current state of %s: %w", articleID, err)
	}
	if !found || !e.cfg.Target.matches(after) {
		return outcomeRebuilt, fmt.Errorf(
			"article %s reported success but its current version is chunker=%q embedder=%q, not the target chunker=%q embedder=%q",
			articleID, after.ChunkerVersion, after.EmbedderVersion,
			e.cfg.Target.ChunkerVersion, e.cfg.Target.EmbedderVersion,
		)
	}

	return outcomeRebuilt, nil
}

func (e *RebuildEngine) reportProgress(ctx context.Context, start time.Time) {
	if e.cfg.ProgressInterval <= 0 {
		return
	}

	ticker := time.NewTicker(e.cfg.ProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := e.snapshot()
			done := stats.Rebuilt + stats.Skipped + stats.Failed
			elapsed := time.Since(start)
			rate := 0.0
			if elapsed.Seconds() > 0 {
				rate = float64(done) / elapsed.Seconds()
			}

			attrs := []any{
				slog.Int64("rebuilt", stats.Rebuilt),
				slog.Int64("skipped", stats.Skipped),
				slog.Int64("failed", stats.Failed),
				slog.Duration("elapsed", elapsed),
				slog.Float64("docs_per_sec", rate),
			}
			if e.cfg.TotalJobs > 0 {
				remaining := e.cfg.TotalJobs - done
				if remaining < 0 {
					remaining = 0
				}
				attrs = append(attrs, slog.Int64("remaining", remaining))
				if rate > 0 {
					attrs = append(attrs, slog.Float64("eta_seconds", float64(remaining)/rate))
				}
			}

			e.logger.Info("rebuild_progress", attrs...)
		}
	}
}

func (e *RebuildEngine) recordFailure(err error) {
	if e.cfg.MaxConsecutiveFailures <= 0 {
		return
	}
	if e.consecutiveFailures.Add(1) < int64(e.cfg.MaxConsecutiveFailures) {
		return
	}
	e.abortOnce.Do(func() {
		e.abortErr = fmt.Errorf("rebuild aborted after %d consecutive failures: %w", e.cfg.MaxConsecutiveFailures, err)
		e.abortFlag.Store(true)
		e.logger.Error("rebuild_aborted",
			slog.Int("consecutive_failures", e.cfg.MaxConsecutiveFailures),
			slog.String("error", err.Error()),
		)
	})
}

func (e *RebuildEngine) snapshot() RebuildStats {
	return RebuildStats{
		Rebuilt: e.rebuilt.Load(),
		Skipped: e.skipped.Load(),
		Failed:  e.failed.Load(),
	}
}

func (e *RebuildEngine) markActivity() {
	e.lastActivity.Store(time.Now().UnixNano())
}

func (e *RebuildEngine) lastActivityAt() time.Time {
	return time.Unix(0, e.lastActivity.Load())
}

// payloadString reads a required string field. A missing field is a malformed
// job, not an empty value to carry into the index.
func payloadString(payload map[string]interface{}, key string) (string, error) {
	raw, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("job payload is missing %q", key)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("job payload field %q is %T, want string", key, raw)
	}
	return value, nil
}

// sleepCtx waits for d and reports whether the wait completed rather than the
// context ending.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
