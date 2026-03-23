package main

import (
	"context"
	"knowledge-sovereign/config"
	"knowledge-sovereign/handler"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	// Metrics / health server
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/health", handler.HealthHandler)
	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metricsMux}

	go func() {
		slog.Info("metrics server starting", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Main RPC server (placeholder for Connect-RPC)
	mainMux := http.NewServeMux()
	mainMux.HandleFunc("/health", handler.HealthHandler)
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
