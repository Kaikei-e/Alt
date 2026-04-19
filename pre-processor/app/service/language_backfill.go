// Package service: language_backfill.go implements a one-shot batch job that
// sweeps the articles table, detects the language of each row still marked
// 'und', and writes the result back. The job is idempotent (re-running is
// safe) and re-entrant (concurrent instances are made safe by the repository's
// `language = 'und'` write predicate, not by this file).
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/repository"
)

// LanguageBackfillConfig configures a single backfill run.
type LanguageBackfillConfig struct {
	BatchSize int
	Throttle  time.Duration
	DryRun    bool
	Logger    *slog.Logger
	// Sleep is injected to keep tests deterministic. When nil, time.Sleep is used.
	Sleep func(time.Duration)
}

// LanguageBackfillSummary reports what the job observed.
type LanguageBackfillSummary struct {
	Scanned     int
	Updated     int
	WouldUpdate int
	SkippedUnd  int
	LastID      string
	ByLanguage  map[string]int
}

// LanguageBackfiller drives the batch loop.
type LanguageBackfiller struct {
	repo   repository.ArticlesLanguageRepo
	cfg    LanguageBackfillConfig
	logger *slog.Logger
	sleep  func(time.Duration)
}

// NewLanguageBackfiller constructs a backfiller with sane defaults.
func NewLanguageBackfiller(repo repository.ArticlesLanguageRepo, cfg LanguageBackfillConfig) *LanguageBackfiller {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	return &LanguageBackfiller{
		repo:   repo,
		cfg:    cfg,
		logger: cfg.Logger,
		sleep:  sleep,
	}
}

// Run iterates cursor-paginated batches until the articles.language='und'
// pool is drained or the context is cancelled. resumeFromID is exclusive: the
// first fetch looks for rows with id > resumeFromID.
func (b *LanguageBackfiller) Run(ctx context.Context, resumeFromID string) (LanguageBackfillSummary, error) {
	summary := LanguageBackfillSummary{
		ByLanguage: make(map[string]int),
		LastID:     resumeFromID,
	}

	afterID := resumeFromID
	for {
		if err := ctx.Err(); err != nil {
			return summary, err
		}

		articles, err := b.repo.FetchUndArticles(ctx, afterID, b.cfg.BatchSize)
		if err != nil {
			return summary, fmt.Errorf("fetch und articles: %w", err)
		}
		if len(articles) == 0 {
			return summary, nil
		}

		updates := make([]repository.LanguageUpdate, 0, len(articles))
		for _, a := range articles {
			summary.Scanned++
			// Title-only detection: article content is stored with HTML markup
			// which inflates the Latin letter count and produces false-positive
			// 'en' classifications. Titles are plain text and match what
			// pre-processor's ingestion path actually passes to the detector.
			detected := DetectLanguage(a.Title)
			summary.ByLanguage[detected]++
			if detected == "und" {
				summary.SkippedUnd++
				continue
			}
			updates = append(updates, repository.LanguageUpdate{ID: a.ID, Language: detected})
		}
		afterID = articles[len(articles)-1].ID
		summary.LastID = afterID

		if b.cfg.DryRun {
			summary.WouldUpdate += len(updates)
			b.logger.InfoContext(ctx, "backfill dry-run batch",
				"batch_size", len(articles),
				"would_update", len(updates),
				"last_id", summary.LastID,
			)
		} else if len(updates) > 0 {
			n, err := b.repo.UpdateLanguageBulk(ctx, updates)
			if err != nil {
				return summary, fmt.Errorf("update language bulk: %w", err)
			}
			summary.Updated += n
			b.logger.InfoContext(ctx, "backfill batch applied",
				"batch_size", len(articles),
				"updated", n,
				"last_id", summary.LastID,
			)
		}

		if b.cfg.Throttle > 0 {
			b.sleep(b.cfg.Throttle)
		}
	}
}
