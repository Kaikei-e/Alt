package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"pre-processor-sidecar/config"
)

func main() {
	// Parse command line flags
	healthCheck := flag.Bool("health-check", false, "Perform health check and exit")
	flag.Parse()

	// Setup structured logging
	logLevel := os.Getenv("LOG_LEVEL")
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	if *healthCheck {
		performHealthCheck()
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("Pre-processor-sidecar CronJob starting", 
		"service", cfg.ServiceName,
		"sync_interval", cfg.RateLimit.SyncInterval,
		"api_daily_limit", cfg.RateLimit.DailyLimit)

	// Run the CronJob task
	ctx := context.Background()
	if err := runCronJobTask(ctx, cfg, logger); err != nil {
		logger.Error("CronJob task failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Pre-processor-sidecar CronJob completed successfully")
}

func performHealthCheck() {
	// Simple health check for CronJob
	fmt.Println("OK")
	os.Exit(0)
}

func runCronJobTask(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	// TODO: Create actual service implementations
	// For now, this is a placeholder structure
	logger.Info("CronJob task would execute here")
	logger.Info("Configuration loaded",
		"inoreader_base_url", cfg.Inoreader.BaseURL,
		"max_articles_per_request", cfg.Inoreader.MaxArticlesPerRequest,
		"sync_interval", cfg.RateLimit.SyncInterval)

	// Simulate successful execution
	time.Sleep(2 * time.Second)
	
	return nil
}