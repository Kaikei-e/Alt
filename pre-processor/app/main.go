package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"pre-processor/driver"
	"pre-processor/handler"
	"pre-processor/repository"
	"pre-processor/service"
	logger "pre-processor/utils/logger"
)

const (
	BATCH_SIZE       = 40
	NEWS_CREATOR_URL = "http://news-creator:11434"
)

func main() {
	// Load logger configuration from environment
	config := logger.LoadLoggerConfigFromEnv()

	// Initialize logger with feature flag support
	logFile := os.Stdout
	contextLogger := logger.NewContextLoggerWithConfig(config, logFile)

	// Use context logger as primary logger
	logger.Logger = contextLogger.WithContext(context.Background())
	logger.Logger.Info("Starting pre-processor service",
		"log_level", config.Level,
		"log_format", config.Format,
		"use_rask_logger", config.UseRask)

	// Initialize database
	ctx := context.Background()

	dbPool, err := driver.Init(ctx)
	if err != nil {
		logger.Logger.Error("Failed to initialize database", "error", err)
		panic(err)
	}

	defer dbPool.Close()

	// Initialize repositories
	articleRepo := repository.NewArticleRepository(dbPool, logger.Logger)
	feedRepo := repository.NewFeedRepository(dbPool, logger.Logger)
	summaryRepo := repository.NewSummaryRepository(dbPool, logger.Logger)
	apiRepo := repository.NewExternalAPIRepository(logger.Logger)

	// Initialize services
	feedProcessorService := service.NewFeedProcessorService(
		feedRepo,
		articleRepo,
		service.NewArticleFetcherService(logger.Logger),
		logger.Logger,
	)

	articleSummarizerService := service.NewArticleSummarizerService(
		articleRepo,
		summaryRepo,
		apiRepo,
		logger.Logger,
	)

	qualityCheckerService := service.NewQualityCheckerService(
		summaryRepo,
		apiRepo,
		dbPool,
		logger.Logger,
	)

	healthCheckerService := service.NewHealthCheckerService(
		NEWS_CREATOR_URL,
		logger.Logger,
	)

	// Initialize health metrics collector
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	// Initialize handlers
	jobHandler := handler.NewJobHandler(
		feedProcessorService,
		articleSummarizerService,
		qualityCheckerService,
		healthCheckerService,
		BATCH_SIZE,
		logger.Logger,
	)

	healthHandler := handler.NewHealthHandler(
		healthCheckerService,
		metricsCollector,
		logger.Logger,
	)

	jobScheduler := handler.NewJobScheduler(logger.Logger)

	// Start jobs
	logger.Logger.Info("Starting background jobs")

	// Start feed processing job
	if err := jobHandler.StartFeedProcessingJob(ctx); err != nil {
		logger.Logger.Error("Failed to start feed processing job", "error", err)
		panic(err)
	}

	// Start summarization job
	if err := jobHandler.StartSummarizationJob(ctx); err != nil {
		logger.Logger.Error("Failed to start summarization job", "error", err)
		panic(err)
	}

	// Start quality check job
	if err := jobHandler.StartQualityCheckJob(ctx); err != nil {
		logger.Logger.Error("Failed to start quality check job", "error", err)
		panic(err)
	}

	// Check health of dependencies
	logger.Logger.Info("Checking health of dependencies")

	if err := healthHandler.CheckDependencies(ctx); err != nil {
		logger.Logger.Warn("Some dependencies are not healthy", "error", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	logger.Logger.Info("Pre-processor service started successfully")
	logger.Logger.Info("Press Ctrl+C to stop")

	<-quit
	logger.Logger.Info("Shutting down pre-processor service")

	// Stop all jobs
	if err := jobHandler.Stop(); err != nil {
		logger.Logger.Error("Error stopping job handler", "error", err)
	}

	if err := jobScheduler.StopAll(); err != nil {
		logger.Logger.Error("Error stopping job scheduler", "error", err)
	}

	logger.Logger.Info("Pre-processor service stopped")
}
