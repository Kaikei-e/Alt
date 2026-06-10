package main

import (
	"context"
	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
	"knowledge-sovereign/usecase/act_outcome_cron"
	"knowledge-sovereign/usecase/knowledge_loop_projector"
	"knowledge-sovereign/usecase/projection_health"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Database connection
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected")

	// Initialize layers
	repo := sovereign_db.NewRepository(pool)
	sovereignHandler := handler.NewSovereignHandler(repo, handler.WithDatabaseURL(cfg.DatabaseURL))

	// Snapshot handler
	snapshotDir := os.Getenv("SNAPSHOT_DIR")
	if snapshotDir == "" {
		snapshotDir = "/data/snapshots"
	}
	buildRef := os.Getenv("BUILD_REF")
	if buildRef == "" {
		buildRef = "dev"
	}
	snapshotHandler := handler.NewSnapshotHandler(repo, snapshotDir, buildRef, "00009")
	archiveDir := os.Getenv("ARCHIVE_DIR")
	if archiveDir == "" {
		archiveDir = "/tmp/archives"
	}
	retentionHandler := handler.NewRetentionHandler(repo, archiveDir)
	storageHandler := handler.NewStorageHandler(repo)
	knowledgeLoopReprojectHandler := handler.NewKnowledgeLoopReprojectHandler(repo)

	// Metrics / health server
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/health", handler.HealthHandler)
	// Prometheus scrape endpoint. Default registry collectors include
	// process / Go runtime metrics out of the box; the Knowledge Loop
	// projector counters in usecase/knowledge_loop_projector/metrics.go
	// register with the same default registry via promauto.
	metricsMux.Handle("/metrics", promhttp.Handler())
	snapshotHandler.RegisterRoutes(metricsMux)
	retentionHandler.RegisterRoutes(metricsMux)
	storageHandler.RegisterRoutes(metricsMux)
	knowledgeLoopReprojectHandler.RegisterRoutes(metricsMux)
	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metricsMux}

	go func() {
		slog.Info("metrics server starting", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Main RPC server with Connect-RPC handlers
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/health", handler.HealthHandler)

	path, rpcHandler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(sovereignHandler)
	mainMux.Handle(path, rpcHandler)

	mainServer := &http.Server{Addr: cfg.ListenAddr, Handler: mainMux}

	go func() {
		slog.Info("rpc server starting", "addr", cfg.ListenAddr)
		if err := mainServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("rpc server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Knowledge Loop projector. Runs in-process now that the projection logic
	// has moved here (ADR-000844 follow-up). The cadence is intentionally short
	// — the projector is reproject-safe and idempotent, so re-running on a
	// quiet log is cheap.
	//
	// ADR-000914 §projector performance: BatchSize and MaxBatchesPerTick are
	// env-tunable so reproject can be driven by bumping the env in the
	// sovereign deployment without a redeploy of the projector code. The
	// defaults match the constants in `projector.go` (500 × 4 batches per
	// tick = 2 000 events / tick).
	loopProjector := knowledge_loop_projector.NewProjector(repo, slog.Default(),
		knowledge_loop_projector.Config{
			BatchSize:         parseIntEnv("KNOWLEDGE_SOVEREIGN_LOOP_PROJECTOR_BATCH_SIZE", 500),
			MaxBatchesPerTick: parseIntEnv("KNOWLEDGE_SOVEREIGN_LOOP_PROJECTOR_MAX_BATCHES_PER_TICK", 4),
		})
	// ADR-000939: the projector derives evidence from the co-projected
	// knowledge_loop_evidence accumulator it folds in the same pass — there is
	// no separate resolver to wire. `repo` (sovereign_db.Repository) provides
	// the accumulator methods; a missing implementation would be a compile error.
	loopTick := time.NewTicker(parseDurationEnv("KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL", 5*time.Second))
	go func() {
		defer loopTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-loopTick.C:
				if err := loopProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_loop_projector batch failed", "error", err)
				}
			}
		}
	}()

	// ADR-000939 honest gate: sample the relation-coverage and producer-liveness
	// gauges on a slow tick. DB-truth gauges replace the old rate-based coverage
	// alert that could never fire at the real projection traffic.
	healthExporter := projection_health.New(repo, slog.Default())
	healthTick := time.NewTicker(parseDurationEnv("KNOWLEDGE_SOVEREIGN_PROJECTION_HEALTH_TICK_INTERVAL", 60*time.Second))
	go func() {
		defer healthTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-healthTick.C:
				if err := healthExporter.RunOnce(ctx); err != nil {
					slog.Error("projection_health exporter failed", "error", err)
				}
			}
		}
	}()

	// ADR-000939: surface_planner_cron is retired. It existed to re-emit
	// KnowledgeLoopSurfacePlanRecomputed for AugurConversationLinked signals so
	// the projector had a source, but that re-emit layer is a detour now that
	// the projector consumes augur links directly as late fuel and derives
	// placement from the co-projected evidence accumulator. The consumer
	// projector branch is kept (44 historical surface_plan_recomputed events
	// replay deterministically), but nothing emits new ones. The stale
	// `surface_planner_v2` projection checkpoint row is removed during the
	// rollout (see docs/runbooks/knowledge-loop-reproject.md) so its heartbeat
	// SLO does not false-fire.

	// ADR-000908 §Δ1 act_outcome_cron. Backfills `outcome=no_engagement` for
	// KnowledgeLoopActed events that aged past the 7-day window without an
	// explicit outcome from the alt-backend view tracker. Reproject-safe —
	// the emitted outcome event's occurred_at is bound to
	// acted.occurred_at + 7d, not wall-clock. The cadence is loose because
	// the cutoff itself is in days; one tick per hour catches the boundary
	// promptly enough.
	outcomeCron := act_outcome_cron.New(repo, slog.Default(), act_outcome_cron.Config{
		BatchSize: parseIntEnv("KNOWLEDGE_SOVEREIGN_ACT_OUTCOME_BATCH_SIZE", 256),
		// Identifier-use only: scan boundary for "which acted events have
		// aged past the 7d cutoff". The emitted event payloads remain pure
		// (occurred_at = acted.OccurredAt + Window) regardless of when
		// Clock() fires. See act_outcome_cron/invariants_test.go.
		Clock: time.Now,
	})
	outcomeTick := time.NewTicker(parseDurationEnv("KNOWLEDGE_SOVEREIGN_ACT_OUTCOME_TICK_INTERVAL", 1*time.Hour))
	go func() {
		defer outcomeTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-outcomeTick.C:
				if err := outcomeCron.RunBatch(ctx); err != nil {
					slog.Error("act_outcome_cron batch failed", "error", err)
				}
			}
		}
	}()

	slog.Info("knowledge-sovereign started",
		"listen", cfg.ListenAddr,
		"metrics", cfg.MetricsAddr)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	metricsServer.Shutdown(shutdownCtx)
	mainServer.Shutdown(shutdownCtx)
	slog.Info("shutdown complete")
}

// parseDurationEnv reads a duration from env, falling back to the supplied
// default. Negative or unparseable values fall back without error so a
// misconfigured operator override does not crash the service.
func parseDurationEnv(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		slog.Warn("invalid duration env, using fallback", "env", name, "value", v, "fallback", fallback.String())
		return fallback
	}
	return d
}

func parseIntEnv(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		slog.Warn("invalid int env, using fallback", "env", name, "value", v, "fallback", fallback)
		return fallback
	}
	return i
}
