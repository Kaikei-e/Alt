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
