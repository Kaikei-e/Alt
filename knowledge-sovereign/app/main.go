package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/handler"
	"knowledge-sovereign/internal/pki"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// config.Load builds the auth-hub mTLS client, which needs the leaf on disk.
	pkiHandle, err := pki.Start(ctx, logger, "knowledge-sovereign")
	if err != nil {
		slog.Error("pki enrollment failed", "error_type", pki.LogSafeError(err))
		os.Exit(1)
	}
	defer pkiHandle.Stop()
	opsSrv, err := pki.ListenOps(ctx, logger, "knowledge-sovereign", pkiHandle.MetricsHandler())
	if err != nil {
		slog.Error("pki ops listener failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		opsCtx, opsCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer opsCancel()
		_ = pki.ShutdownOps(opsCtx, opsSrv)
	}()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

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

	repo := sovereign_db.NewRepository(pool)
	sovereignHandler := handler.NewSovereignHandler(repo, handler.WithDatabaseURL(cfg.DatabaseURL))

	servers := startServers(cfg, repo, sovereignHandler)

	var wg sync.WaitGroup
	startWorkers(ctx, &wg, repo, cfg)

	slog.Info("knowledge-sovereign started",
		"listen", cfg.ListenAddr,
		"metrics", cfg.MetricsAddr)

	waitForShutdown(ctx, cancel, &wg, servers)
}

func waitForShutdown(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, servers *serverGroup) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	servers.shutdown(shutdownCtx)

	cancel()
	wg.Wait()
	slog.Info("shutdown complete")
}
