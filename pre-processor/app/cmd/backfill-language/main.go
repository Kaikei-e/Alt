// Command backfill-language populates articles.language for rows whose value
// is still the default 'und'. It is a one-shot administrative job: run it
// once after the ADR-000776 migration lands and articles flow through
// language-aware ingestion on their own.
//
// Usage:
//
//	backfill-language \
//	  --dsn "postgres://user:pass@host:5432/alt?sslmode=disable" \
//	  --batch-size 500 --throttle-ms 100 [--dry-run] [--resume-from-id ID]
//
// The DSN can also be provided via ARTICLES_DB_DSN or DATABASE_URL env vars.
// On SIGINT/SIGTERM the job finishes the in-flight batch and exits cleanly;
// the next run resumes from the last committed batch because the WHERE
// predicate filters on language='und'.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pre-processor/repository"
	"pre-processor/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

type options struct {
	DSN         string
	BatchSize   int
	ThrottleMs  int
	DryRun      bool
	ResumeFrom  string
	ConnTimeout time.Duration
}

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("backfill-language", flag.ContinueOnError)
	var opts options
	fs.StringVar(&opts.DSN, "dsn", "", "PostgreSQL DSN (defaults to ARTICLES_DB_DSN / DATABASE_URL)")
	fs.IntVar(&opts.BatchSize, "batch-size", 500, "rows per batch")
	fs.IntVar(&opts.ThrottleMs, "throttle-ms", 100, "milliseconds to sleep between batches")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "detect but do not update")
	fs.StringVar(&opts.ResumeFrom, "resume-from-id", "", "exclusive lower bound on article id for cursor-paginated resume")
	fs.DurationVar(&opts.ConnTimeout, "connect-timeout", 30*time.Second, "connect timeout for initial DB handshake")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	if opts.DSN == "" {
		opts.DSN = os.Getenv("ARTICLES_DB_DSN")
	}
	if opts.DSN == "" {
		opts.DSN = os.Getenv("DATABASE_URL")
	}
	if opts.DSN == "" {
		return options{}, errors.New("DSN is required (flag --dsn, ARTICLES_DB_DSN, or DATABASE_URL)")
	}
	if opts.BatchSize <= 0 {
		return options{}, fmt.Errorf("invalid --batch-size=%d", opts.BatchSize)
	}
	if opts.ThrottleMs < 0 {
		return options{}, fmt.Errorf("invalid --throttle-ms=%d", opts.ThrottleMs)
	}
	return opts, nil
}

func run(ctx context.Context, opts options, logger *slog.Logger) (service.LanguageBackfillSummary, error) {
	connectCtx, cancel := context.WithTimeout(ctx, opts.ConnTimeout)
	defer cancel()

	pool, err := pgxpool.New(connectCtx, opts.DSN)
	if err != nil {
		return service.LanguageBackfillSummary{}, fmt.Errorf("connect to DB: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(connectCtx); err != nil {
		return service.LanguageBackfillSummary{}, fmt.Errorf("ping DB: %w", err)
	}

	repo := repository.NewArticlesLanguageRepo(pool, logger)
	backfiller := service.NewLanguageBackfiller(repo, service.LanguageBackfillConfig{
		BatchSize: opts.BatchSize,
		Throttle:  time.Duration(opts.ThrottleMs) * time.Millisecond,
		DryRun:    opts.DryRun,
		Logger:    logger,
	})

	return backfiller.Run(ctx, opts.ResumeFrom)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		logger.Error("argument error", "err", err.Error())
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	logger.Info("backfill-language: starting",
		"batch_size", opts.BatchSize,
		"throttle_ms", opts.ThrottleMs,
		"dry_run", opts.DryRun,
		"resume_from", opts.ResumeFrom,
	)

	summary, err := run(ctx, opts, logger)
	elapsed := time.Since(start)

	logger.Info("backfill-language: finished",
		"scanned", summary.Scanned,
		"updated", summary.Updated,
		"would_update", summary.WouldUpdate,
		"skipped_und", summary.SkippedUnd,
		"last_id", summary.LastID,
		"by_language", summary.ByLanguage,
		"elapsed", elapsed.String(),
	)

	if err != nil {
		logger.Error("backfill-language: failed", "err", err.Error())
		os.Exit(1)
	}
}
