package main

import (
	"context"
	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
	"knowledge-sovereign/usecase/knowledge_trail_projector"
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

	// Knowledge Trail spine projector. Folds the append-only event log into
	// knowledge_trail_footprints in-process. Reproject-safe and idempotent, so a
	// short tick over a quiet log is cheap.
	trailProjector := knowledge_trail_projector.NewProjector(repo, slog.Default(),
		knowledge_trail_projector.Config{
			BatchSize:         parseIntEnv("KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_BATCH_SIZE", 500),
			MaxBatchesPerTick: parseIntEnv("KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_MAX_BATCHES_PER_TICK", 4),
		})
	trailTick := time.NewTicker(parseDurationEnv("KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL", 5*time.Second))
	go func() {
		defer trailTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-trailTick.C:
				if err := trailProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_trail_projector batch failed", "error", err)
				}
			}
		}
	}()

	// Producer-liveness gauges sampled on a slow tick.
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
