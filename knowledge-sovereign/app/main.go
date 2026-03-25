package main

import (
	"context"
	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/gen/proto/services/sovereign/v1/sovereignv1connect"
	"knowledge-sovereign/handler"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
