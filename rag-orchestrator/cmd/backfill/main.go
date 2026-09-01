package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"rag-orchestrator/internal/adapter/rag_augur"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/backfill"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/httpclient"
	"rag-orchestrator/internal/usecase"
)

var (
	version = "dev"

	// Global flags
	verbose    bool
	cursorFile string

	// Run command flags
	fromDate    string
	toDate      string
	concurrency int
	batchSize   int
	dryRun      bool
	hyperBoost  bool
	directMode  bool

	// Rebuild command flags
	rebuildWorkers  int
	rebuildAll      bool
	rebuildDryRun   bool
	rebuildPageSize int
)

// rebuildEmbedderTimeoutSeconds is generous on purpose: a rebuild embeds whole
// documents in one call and has no interactive caller waiting on it.
const rebuildEmbedderTimeoutSeconds = 300

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:     "backfill",
	Short:   "Backfill articles to RAG index",
	Version: version,
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the backfill process",
	Long: `Run the backfill process to index articles into the RAG system.

The process can be resumed from where it left off using cursor tracking.
Use --from and --to to specify a date range, or run without flags to
process all articles.

Examples:
  # Process all articles (resumes from cursor)
  backfill run

  # Process articles from a specific date range
  backfill run --from 2024-01-01 --to 2024-01-31

  # Dry run to see what would be processed
  backfill run --from 2024-12-01 --dry-run

  # Adjust concurrency
  backfill run --concurrency 4`,
	RunE: runBackfill,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current cursor status",
	RunE:  showStatus,
}

var resetCmd = &cobra.Command{
	Use:   "reset-cursor",
	Short: "Reset the cursor to start from beginning",
	RunE:  resetCursor,
}

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "One-shot full re-index onto a new chunker / embedder",
	Long: `Rebuild the whole index onto a new chunker and embedding model.

The work is queued in rag_jobs and drained by a bounded pool of workers, so a
run survives a restart: the queue and the recorded per-document chunker and
embedder versions are the state, not a local cursor file.

Required environment:
  RAG_DB_URL        rag-db connection string
  DATABASE_URL      source database (articles) — enqueue only
  EMBEDDING_MODEL   the model chosen by evaluation; there is no default
  EMBEDDER_URL      embedder base URL, or EMBEDDER_URLS for a comma-separated
                    list of replicas serving that same model

Typical run:
  backfill rebuild enqueue --dry-run
  backfill rebuild enqueue
  backfill rebuild run --workers 3`,
}

var rebuildEnqueueCmd = &cobra.Command{
	Use:   "enqueue",
	Short: "Queue one job per source article that is not already at the target",
	RunE:  runRebuildEnqueue,
	// A missing setting or an unreachable database is not a usage error, and
	// dumping the flag list on top of it buries the message.
	SilenceUsage: true,
}

var rebuildRunCmd = &cobra.Command{
	Use:          "run",
	Short:        "Drain the rebuild queue with bounded parallel workers",
	RunE:         runRebuildDrain,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&cursorFile, "cursor-file", "cursor.json", "cursor file path")

	runCmd.Flags().StringVar(&fromDate, "from", "", "start date (YYYY-MM-DD)")
	runCmd.Flags().StringVar(&toDate, "to", "", "end date (YYYY-MM-DD), defaults to today")
	runCmd.Flags().IntVar(&concurrency, "concurrency", 4, "number of concurrent requests")
	runCmd.Flags().IntVar(&batchSize, "batch-size", 40, "articles per batch")
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be processed without actually processing")
	runCmd.Flags().BoolVar(&hyperBoost, "hyper-boost", false, "use local GPU for embedding (starts temporary Ollama container)")
	runCmd.Flags().BoolVar(&directMode, "direct", false, "bypass HTTP, index directly via rag-db + embedder (requires RAG_DB_URL, EMBEDDER_URL)")

	rebuildEnqueueCmd.Flags().BoolVar(&rebuildDryRun, "dry-run", false, "scan and report without writing any job")
	rebuildEnqueueCmd.Flags().BoolVar(&rebuildAll, "all", false, "queue every article, including ones already at the target")
	rebuildEnqueueCmd.Flags().IntVar(&rebuildPageSize, "page-size", 500, "source rows fetched per keyset page")

	rebuildRunCmd.Flags().IntVar(&rebuildWorkers, "workers", backfill.DefaultRebuildConfig().Workers,
		"documents embedded concurrently")

	rebuildCmd.AddCommand(rebuildEnqueueCmd)
	rebuildCmd.AddCommand(rebuildRunCmd)

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(rebuildCmd)
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

func runBackfill(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	orchestratorURL := os.Getenv("ORCHESTRATOR_URL")
	if orchestratorURL == "" {
		orchestratorURL = "http://localhost:9010"
	}

	cfg := backfill.DefaultConfig()
	cfg.DatabaseURL = dbURL
	cfg.OrchestratorURL = orchestratorURL
	cfg.CursorFile = cursorFile
	cfg.Concurrency = concurrency
	cfg.BatchSize = batchSize
	cfg.DryRun = dryRun

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle hyper-boost mode
	var hb *backfill.HyperBoost
	if hyperBoost {
		logger.Info("initializing hyper-boost mode")

		model, err := rebuildEmbeddingModel()
		if err != nil {
			return err
		}

		hb, err = backfill.NewHyperBoost(logger, model)
		if err != nil {
			return fmt.Errorf("create hyperboost: %w", err)
		}
		defer func() {
			if stopErr := hb.Stop(context.Background()); stopErr != nil {
				logger.Warn("failed to stop hyperboost container", slog.String("error", stopErr.Error()))
			}
			_ = hb.Close()
		}()

		if err := hb.Start(ctx); err != nil {
			return fmt.Errorf("start hyperboost container: %w", err)
		}

		if err := hb.WaitReady(ctx); err != nil {
			return fmt.Errorf("hyperboost container not ready: %w", err)
		}

		if err := hb.PullModel(ctx); err != nil {
			return fmt.Errorf("pull embedding model: %w", err)
		}

		cfg.EmbedderOverrideURL = hb.EmbedderURL()
		logger.Info("hyper-boost enabled",
			slog.String("embedder_url", cfg.EmbedderOverrideURL),
		)
	}

	// Handle direct mode
	if directMode {
		ragDBURL := os.Getenv("RAG_DB_URL")
		if ragDBURL == "" {
			return fmt.Errorf("RAG_DB_URL environment variable is required for --direct mode")
		}
		// No default model: an unset EMBEDDING_MODEL used to fall back to a
		// hardcoded name, which writes the corpus into whichever vector space
		// that name happens to serve today rather than the one that was chosen.
		embeddingModel, err := rebuildEmbeddingModel()
		if err != nil {
			return err
		}

		// Support multiple embedder replicas (EMBEDDER_URLS) or single (EMBEDDER_URL)
		var embedderURLs []string
		if urls := os.Getenv("EMBEDDER_URLS"); urls != "" {
			embedderURLs = strings.Split(urls, ",")
		} else if url := os.Getenv("EMBEDDER_URL"); url != "" {
			embedderURLs = []string{url}
		} else {
			return fmt.Errorf("EMBEDDER_URL or EMBEDDER_URLS environment variable is required for --direct mode")
		}

		// Connect to rag-db with pgvector type registration
		ragPool, err := infra.NewPostgresDB(ctx, ragDBURL)
		if err != nil {
			return fmt.Errorf("connect to rag-db: %w", err)
		}
		defer ragPool.Close()

		// Shared DB dependencies
		chunkRepo := repository.NewRagChunkRepository(ragPool)
		docRepo := repository.NewRagDocumentRepository(ragPool)
		txManager := repository.NewPostgresTransactionManager(ragPool)
		hasher := domain.NewSourceHashPolicy()
		chunker := domain.NewChunker()

		// Build one IndexArticleUsecase per embedder replica
		var indexers []usecase.IndexArticleUsecase
		for _, eURL := range embedderURLs {
			embedder := rag_augur.NewOllamaEmbedder(
				strings.TrimSpace(eURL), embeddingModel, 120, logger,
				httpclient.NewPooledClient(120*time.Second),
			)
			indexers = append(indexers, usecase.NewIndexArticleUsecase(
				docRepo, chunkRepo, txManager, hasher, chunker, embedder,
			))
		}

		cfg.Direct = true
		if len(indexers) == 1 {
			cfg.DirectIndexer = backfill.NewDirectIndexer(indexers[0], concurrency, logger)
		} else {
			cfg.DirectIndexer = backfill.NewDirectIndexerMulti(indexers, concurrency, logger)
		}

		logger.Info("direct mode enabled",
			slog.String("rag_db", ragDBURL),
			slog.Int("embedder_replicas", len(embedderURLs)),
			slog.String("embedding_model", embeddingModel),
		)
	}

	// Parse dates
	if fromDate != "" {
		t, err := time.Parse("2006-01-02", fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		cfg.FromDate = t
	}

	if toDate != "" {
		t, err := time.Parse("2006-01-02", toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		cfg.ToDate = t
	}

	logger.Info("starting backfill",
		slog.String("orchestrator_url", cfg.OrchestratorURL),
		slog.String("cursor_file", cfg.CursorFile),
		slog.Int("concurrency", cfg.Concurrency),
		slog.Int("batch_size", cfg.BatchSize),
		slog.Bool("dry_run", cfg.DryRun),
		slog.Bool("hyper_boost", hyperBoost),
		slog.String("from_date", fromDate),
		slog.String("to_date", toDate),
	)

	runner, err := backfill.NewRunner(cfg, logger)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer func() { _ = runner.Close() }()

	// Setup signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down...", slog.String("signal", sig.String()))
		cancel()
	}()

	if err := runner.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("backfill interrupted, cursor saved for resume")
			return nil
		}
		return fmt.Errorf("run backfill: %w", err)
	}

	return nil
}

func showStatus(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	cfg := backfill.DefaultConfig()
	cfg.CursorFile = cursorFile
	cfg.DatabaseURL = "postgres://dummy:dummy@localhost/dummy" // Not used for status

	runner, err := backfill.NewRunner(cfg, logger)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer func() { _ = runner.Close() }()

	cursor, err := runner.GetCursor()
	if err != nil {
		return fmt.Errorf("get cursor: %w", err)
	}

	if cursor.IsEmpty() {
		fmt.Println("No cursor found. Backfill will start from the beginning.")
		return nil
	}

	fmt.Printf("Cursor Status:\n")
	fmt.Printf("  Version:         %d\n", cursor.Version)
	fmt.Printf("  Last Created At: %s\n", cursor.LastCreatedAt.Format(time.RFC3339))
	fmt.Printf("  Last ID:         %s\n", cursor.LastID)
	fmt.Printf("  Current Date:    %s\n", cursor.CurrentDate)
	fmt.Printf("  Processed Count: %d\n", cursor.ProcessedCount)
	fmt.Printf("  Updated At:      %s\n", cursor.UpdatedAt.Format(time.RFC3339))

	return nil
}

func resetCursor(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	cfg := backfill.DefaultConfig()
	cfg.CursorFile = cursorFile
	cfg.DatabaseURL = "postgres://dummy:dummy@localhost/dummy" // Not used for reset

	runner, err := backfill.NewRunner(cfg, logger)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer func() { _ = runner.Close() }()

	if err := runner.ResetCursor(); err != nil {
		return fmt.Errorf("reset cursor: %w", err)
	}

	logger.Info("cursor reset successfully")
	return nil
}

// requireEnv reads a mandatory setting. A rebuild that infers a missing value
// is how a corpus ends up half-written into a vector space nobody chose.
func requireEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

// rebuildEmbeddingModel reads the embedding model. It deliberately has no
// default: the model is chosen by evaluation, and a wrong guess here is only
// visible hours later as a corpus embedded in the wrong vector space.
func rebuildEmbeddingModel() (string, error) {
	return requireEnv("EMBEDDING_MODEL")
}

// resolveEmbedderURLs reads the embedder replica list, preferring the
// comma-separated EMBEDDER_URLS over the single-URL EMBEDDER_URL.
func resolveEmbedderURLs() ([]string, error) {
	raw := os.Getenv("EMBEDDER_URLS")
	if strings.TrimSpace(raw) == "" {
		single, err := requireEnv("EMBEDDER_URL")
		if err != nil {
			return nil, fmt.Errorf("EMBEDDER_URL or EMBEDDER_URLS is required")
		}
		return []string{single}, nil
	}

	var urls []string
	for _, candidate := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("EMBEDDER_URLS is set but contains no URL")
	}
	return urls, nil
}

// singleEmbedderVersion returns the vector space every replica agrees on.
// Replicas serving different models would write two incompatible spaces into
// one column, and the rebuild's target could only ever match one of them.
func singleEmbedderVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no embedder replica configured")
	}
	for _, version := range versions[1:] {
		if version != versions[0] {
			return "", fmt.Errorf("embedder replicas report different models: %s", strings.Join(versions, ", "))
		}
	}
	return versions[0], nil
}

// rebuildDeps holds the rag-db side of a rebuild command.
type rebuildDeps struct {
	pool     *pgxpool.Pool
	jobs     domain.RagJobRepository
	state    backfill.VersionStateReader
	indexers []usecase.IndexArticleUsecase
	target   backfill.RebuildTarget
}

func (d *rebuildDeps) Close() {
	d.pool.Close()
}

// newRebuildDeps connects rag-db and builds one index usecase per embedder
// replica.
//
// The target is read off the very components that will write it —
// chunker.Version() and embedder.Version() — rather than from a separately
// configured string. A hand-typed target that drifts from what the indexer
// records would fail the post-write verification on every single document.
func newRebuildDeps(ctx context.Context, logger *slog.Logger) (*rebuildDeps, error) {
	ragDBURL, err := requireEnv("RAG_DB_URL")
	if err != nil {
		return nil, err
	}
	model, err := rebuildEmbeddingModel()
	if err != nil {
		return nil, err
	}
	urls, err := resolveEmbedderURLs()
	if err != nil {
		return nil, err
	}

	pool, err := infra.NewPostgresDB(ctx, ragDBURL)
	if err != nil {
		return nil, fmt.Errorf("connect to rag-db: %w", err)
	}

	chunkRepo := repository.NewRagChunkRepository(pool)
	docRepo := repository.NewRagDocumentRepository(pool)
	txManager := repository.NewPostgresTransactionManager(pool)
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	var indexers []usecase.IndexArticleUsecase
	var versions []string
	for _, url := range urls {
		embedder := rag_augur.NewOllamaEmbedder(
			url, model, rebuildEmbedderTimeoutSeconds, logger,
			httpclient.NewPooledClient(rebuildEmbedderTimeoutSeconds*time.Second),
		)
		versions = append(versions, embedder.Version())
		indexers = append(indexers, usecase.NewIndexArticleUsecase(
			docRepo, chunkRepo, txManager, hasher, chunker, embedder,
		))
	}

	embedderVersion, err := singleEmbedderVersion(versions)
	if err != nil {
		pool.Close()
		return nil, err
	}

	state, err := backfill.NewPgxVersionStateReader(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	deps := &rebuildDeps{
		pool:     pool,
		jobs:     repository.NewRagJobRepository(pool),
		state:    state,
		indexers: indexers,
		target: backfill.RebuildTarget{
			ChunkerVersion:  string(chunker.Version()),
			EmbedderVersion: embedderVersion,
		},
	}

	logger.Info("rebuild_target_resolved",
		slog.String("chunker_version", deps.target.ChunkerVersion),
		slog.String("embedder_version", deps.target.EmbedderVersion),
		slog.String("embedding_model", model),
		slog.Int("embedder_replicas", len(indexers)),
	)

	return deps, nil
}

func runRebuildEnqueue(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sourceURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}

	deps, err := newRebuildDeps(ctx, logger)
	if err != nil {
		return err
	}
	defer deps.Close()

	sourceDB, err := sql.Open("pgx", sourceURL)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer func() { _ = sourceDB.Close() }()

	source, err := backfill.NewSQLArticleSource(sourceDB, rebuildPageSize)
	if err != nil {
		return err
	}

	cfg := backfill.DefaultEnqueueConfig()
	cfg.Target = deps.target
	cfg.SkipUpToDate = !rebuildAll
	cfg.DryRun = rebuildDryRun

	enqueuer, err := backfill.NewEnqueuer(source, deps.state, deps.jobs, cfg, logger)
	if err != nil {
		return err
	}

	stats, err := enqueuer.Run(ctx)
	if err != nil {
		return fmt.Errorf("enqueue rebuild jobs: %w", err)
	}

	fmt.Printf("scanned %d, enqueued %d, skipped up-to-date %d, skipped empty %d\n",
		stats.Scanned, stats.Enqueued, stats.SkippedUpToDate, stats.SkippedEmpty)

	return nil
}

func runRebuildDrain(cmd *cobra.Command, args []string) error {
	logger := newLogger()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	deps, err := newRebuildDeps(ctx, logger)
	if err != nil {
		return err
	}
	defer deps.Close()

	total, err := backfill.CountPendingJobs(ctx, deps.pool)
	if err != nil {
		return err
	}

	cfg := backfill.DefaultRebuildConfig()
	cfg.Target = deps.target
	cfg.Workers = rebuildWorkers
	cfg.TotalJobs = total

	engine, err := backfill.NewRebuildEngine(deps.jobs, deps.indexers, deps.state, cfg, logger)
	if err != nil {
		return err
	}

	stats, err := engine.Run(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("rebuild interrupted; queued jobs remain for the next run",
				slog.Int64("rebuilt", stats.Rebuilt),
				slog.Int64("skipped", stats.Skipped),
				slog.Int64("failed", stats.Failed),
			)
			return nil
		}
		return err
	}

	if stats.Failed > 0 {
		return fmt.Errorf("rebuild finished with %d failed jobs; rerun `rebuild enqueue` then `rebuild run` to retry them", stats.Failed)
	}

	fmt.Printf("rebuilt %d, skipped %d (already at target)\n", stats.Rebuilt, stats.Skipped)
	return nil
}
