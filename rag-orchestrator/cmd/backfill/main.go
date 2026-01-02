package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	_ "github.com/jackc/pgx/v5/stdlib"

	"rag-orchestrator/internal/backfill"
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
)

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

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVar(&cursorFile, "cursor-file", "cursor.json", "cursor file path")

	runCmd.Flags().StringVar(&fromDate, "from", "", "start date (YYYY-MM-DD)")
	runCmd.Flags().StringVar(&toDate, "to", "", "end date (YYYY-MM-DD), defaults to today")
	runCmd.Flags().IntVar(&concurrency, "concurrency", 4, "number of concurrent requests")
	runCmd.Flags().IntVar(&batchSize, "batch-size", 40, "articles per batch")
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be processed without actually processing")
	runCmd.Flags().BoolVar(&hyperBoost, "hyper-boost", false, "use local GPU for embedding (starts temporary Ollama container)")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(resetCmd)
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

		var err error
		hb, err = backfill.NewHyperBoost(logger)
		if err != nil {
			return fmt.Errorf("create hyperboost: %w", err)
		}
		defer func() {
			if stopErr := hb.Stop(context.Background()); stopErr != nil {
				logger.Warn("failed to stop hyperboost container", slog.String("error", stopErr.Error()))
			}
			hb.Close()
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
	defer runner.Close()

	// Setup signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down...", slog.String("signal", sig.String()))
		cancel()
	}()

	if err := runner.Run(ctx); err != nil {
		if err == context.Canceled {
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
	defer runner.Close()

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
	defer runner.Close()

	if err := runner.ResetCursor(); err != nil {
		return fmt.Errorf("reset cursor: %w", err)
	}

	logger.Info("cursor reset successfully")
	return nil
}
