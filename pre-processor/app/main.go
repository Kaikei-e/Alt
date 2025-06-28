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
	utilsLogger "pre-processor/utils/logger"
)

const (
	BATCH_SIZE       = 40
	NEWS_CREATOR_URL = "http://news-creator:11434"
)

func main() {
	// Load logger configuration from environment
	config := utilsLogger.LoadLoggerConfigFromEnv()

	// Initialize enhanced logger
	logFile := os.Stdout
	contextLogger := utilsLogger.NewContextLogger(logFile, config.Format, config.Level)

	// Use context logger as primary logger
	log := contextLogger.WithContext(context.Background())
	log.Info("Starting pre-processor service",
		"log_level", config.Level,
		"log_format", config.Format)

	// Initialize database
	ctx := context.Background()

	dbPool, err := driver.Init(ctx)
	if err != nil {
		log.Error("Failed to initialize database", "error", err)
		panic(err)
	}

	defer dbPool.Close()

	// Initialize repositories
	articleRepo := repository.NewArticleRepository(dbPool, log)
	feedRepo := repository.NewFeedRepository(dbPool, log)
	summaryRepo := repository.NewSummaryRepository(dbPool, log)
	apiRepo := repository.NewExternalAPIRepository(log)

	// Initialize services
	feedProcessorService := service.NewFeedProcessorService(
		feedRepo,
		articleRepo,
		service.NewArticleFetcherService(log),
		log,
	)

	articleSummarizerService := service.NewArticleSummarizerService(
		articleRepo,
		summaryRepo,
		apiRepo,
		log,
	)

	qualityCheckerService := service.NewQualityCheckerService(
		summaryRepo,
		apiRepo,
		dbPool,
		log,
	)

	healthCheckerService := service.NewHealthCheckerService(
		NEWS_CREATOR_URL,
		log,
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
		log,
	)

	healthHandler := handler.NewHealthHandler(
		healthCheckerService,
		metricsCollector,
		log,
	)

	jobScheduler := handler.NewJobScheduler(log)

	// Start jobs
	log.Info("Starting background jobs")

	// Start feed processing job
	if err := jobHandler.StartFeedProcessingJob(ctx); err != nil {
		log.Error("Failed to start feed processing job", "error", err)
		panic(err)
	}

	// Start summarization job
	if err := jobHandler.StartSummarizationJob(ctx); err != nil {
		log.Error("Failed to start summarization job", "error", err)
		panic(err)
	}

	// Start quality check job
	if err := jobHandler.StartQualityCheckJob(ctx); err != nil {
		log.Error("Failed to start quality check job", "error", err)
		panic(err)
	}

	// Check health of dependencies
	log.Info("Checking health of dependencies")

	if err := healthHandler.CheckDependencies(ctx); err != nil {
		log.Warn("Some dependencies are not healthy", "error", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Info("Pre-processor service started successfully")
	log.Info("Press Ctrl+C to stop")

	<-quit
	log.Info("Shutting down pre-processor service")

	// Stop all jobs
	if err := jobHandler.Stop(); err != nil {
		log.Error("Error stopping job handler", "error", err)
	}

	if err := jobScheduler.StopAll(); err != nil {
		log.Error("Error stopping job scheduler", "error", err)
	}

	log.Info("Pre-processor service stopped")
}
