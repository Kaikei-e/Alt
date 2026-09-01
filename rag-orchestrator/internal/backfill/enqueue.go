package backfill

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// ArticleSource streams the articles a rebuild must cover.
type ArticleSource interface {
	// ScanArticles calls fn once per indexable article. Returning an error from
	// fn stops the scan and surfaces that error.
	ScanArticles(ctx context.Context, fn func(Article) error) error
}

// EnqueueConfig configures filling rag_jobs for a full rebuild.
type EnqueueConfig struct {
	// Target is the derivation the rebuild will produce. Articles already at
	// it are skipped unless SkipUpToDate is off.
	Target RebuildTarget
	// SkipUpToDate keeps a rerun cheap: an article whose current version
	// already matches Target is not queued again.
	SkipUpToDate bool
	// DryRun scans and reports without writing any job.
	DryRun bool
	// LogEvery is how many scanned articles between progress logs.
	LogEvery int64
}

// DefaultEnqueueConfig returns the operational defaults, with Target left
// unset — the caller names the derivation.
func DefaultEnqueueConfig() EnqueueConfig {
	return EnqueueConfig{
		SkipUpToDate: true,
		LogEvery:     1000,
	}
}

// EnqueueStats is the outcome of an enqueue run.
type EnqueueStats struct {
	Scanned         int64
	Enqueued        int64
	SkippedUpToDate int64
	SkippedEmpty    int64
}

// Enqueuer fills rag_jobs with one rebuild job per source article.
type Enqueuer struct {
	source ArticleSource
	state  VersionStateReader
	jobs   domain.RagJobRepository
	cfg    EnqueueConfig
	logger *slog.Logger
}

// NewEnqueuer validates the wiring. The target is mandatory: an enqueue that
// cannot say which derivation it is filling for cannot skip up-to-date work
// either, and would queue the whole corpus on every rerun.
func NewEnqueuer(
	source ArticleSource,
	state VersionStateReader,
	jobs domain.RagJobRepository,
	cfg EnqueueConfig,
	logger *slog.Logger,
) (*Enqueuer, error) {
	if source == nil {
		return nil, fmt.Errorf("enqueuer: article source is required")
	}
	if state == nil {
		return nil, fmt.Errorf("enqueuer: version state reader is required")
	}
	if jobs == nil {
		return nil, fmt.Errorf("enqueuer: job repository is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("enqueuer: logger is required")
	}
	if err := cfg.Target.validate(); err != nil {
		return nil, err
	}

	return &Enqueuer{source: source, state: state, jobs: jobs, cfg: cfg, logger: logger}, nil
}

// Run scans the source and queues a rebuild job for every article whose
// current version is not already at the target.
func (e *Enqueuer) Run(ctx context.Context) (EnqueueStats, error) {
	var stats EnqueueStats

	upToDate := map[string]struct{}{}
	if e.cfg.SkipUpToDate {
		var err error
		upToDate, err = e.state.ArticleIDsAt(ctx, e.cfg.Target)
		if err != nil {
			return stats, fmt.Errorf("read articles already at target: %w", err)
		}
	}

	e.logger.Info("rebuild_enqueue_started",
		slog.String("target_chunker_version", e.cfg.Target.ChunkerVersion),
		slog.String("target_embedder_version", e.cfg.Target.EmbedderVersion),
		slog.Bool("skip_up_to_date", e.cfg.SkipUpToDate),
		slog.Int("already_at_target", len(upToDate)),
		slog.Bool("dry_run", e.cfg.DryRun),
	)

	err := e.source.ScanArticles(ctx, func(a Article) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stats.Scanned++

		if a.Body == "" {
			stats.SkippedEmpty++
			e.logger.Warn("rebuild_enqueue_skipped_empty_body", slog.String("article_id", a.ID))
			return nil
		}
		if _, ok := upToDate[a.ID]; ok {
			stats.SkippedUpToDate++
			return nil
		}

		if !e.cfg.DryRun {
			if err := e.jobs.Enqueue(ctx, newRebuildJob(a)); err != nil {
				return fmt.Errorf("enqueue %s: %w", a.ID, err)
			}
		}
		stats.Enqueued++

		if e.cfg.LogEvery > 0 && stats.Scanned%e.cfg.LogEvery == 0 {
			e.logger.Info("rebuild_enqueue_progress",
				slog.Int64("scanned", stats.Scanned),
				slog.Int64("enqueued", stats.Enqueued),
				slog.Int64("skipped_up_to_date", stats.SkippedUpToDate),
			)
		}
		return nil
	})
	if err != nil {
		return stats, err
	}

	e.logger.Info("rebuild_enqueue_finished",
		slog.Int64("scanned", stats.Scanned),
		slog.Int64("enqueued", stats.Enqueued),
		slog.Int64("skipped_up_to_date", stats.SkippedUpToDate),
		slog.Int64("skipped_empty", stats.SkippedEmpty),
		slog.Bool("dry_run", e.cfg.DryRun),
	)

	return stats, nil
}

func newRebuildJob(a Article) *domain.RagJob {
	now := time.Now()
	return &domain.RagJob{
		ID:      uuid.New(),
		JobType: rebuildJobType,
		Payload: map[string]interface{}{
			"article_id": a.ID,
			"title":      a.Title,
			"url":        a.URL,
			"body":       a.Body,
		},
		Status:    "new",
		CreatedAt: now,
		UpdatedAt: now,
	}
}
