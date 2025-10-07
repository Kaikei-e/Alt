package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pre-processor/config"
	"pre-processor/driver"
	"pre-processor/handler"
	"pre-processor/repository"
	"pre-processor/service"
	logger "pre-processor/utils/logger"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	BATCH_SIZE       = 40
	NEWS_CREATOR_URL = "http://news-creator:11434"
	HTTP_PORT        = "9200" // Default HTTP port for API
)

func main() {
	// Load logger configuration from environment
	loggerConfig := logger.LoadLoggerConfigFromEnv()

	// Initialize logger with feature flag support
	contextLogger := logger.NewContextLoggerWithConfig(loggerConfig)

	// Use context logger as primary logger
	logger.Logger = contextLogger.WithContext(context.Background())
	logger.Logger.Info("Starting pre-processor service",
		"log_level", loggerConfig.Level,
		"log_format", loggerConfig.Format,
		"use_rask_logger", loggerConfig.UseRask)

	// Initialize database
	ctx := context.Background()

	dbPool, err := driver.Init(ctx)
	if err != nil {
		logger.Logger.Error("Failed to initialize database", "error", err)
		panic(err)
	}

	defer dbPool.Close()

	// Load application config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Logger.Error("Failed to load config", "error", err)
		panic(err)
	}

	// Initialize repositories
	articleRepo := repository.NewArticleRepository(dbPool, logger.Logger)
	feedRepo := repository.NewFeedRepository(dbPool, logger.Logger)
	summaryRepo := repository.NewSummaryRepository(dbPool, logger.Logger)
	apiRepo := repository.NewExternalAPIRepository(cfg, logger.Logger)

	// Initialize services
	feedProcessorService := service.NewFeedProcessorService(
		feedRepo,
		articleRepo,
		service.NewArticleFetcherServiceWithFactory(cfg, logger.Logger),
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

	healthCheckerService := service.NewHealthCheckerServiceWithFactory(
		cfg, cfg.NewsCreator.Host, // 設定から正しいURLを使用
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

	// Initialize summarize handler for REST API
	summarizeHandler := handler.NewSummarizeHandler(
		apiRepo,
		summaryRepo,
		logger.Logger,
	)

	jobScheduler := handler.NewJobScheduler(logger.Logger)

	// Initialize Echo HTTP server
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API routes
	api := e.Group("/api/v1")
	api.POST("/summarize", summarizeHandler.HandleSummarize)
	api.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})

	// Start HTTP server in goroutine
	go func() {
		port := os.Getenv("HTTP_PORT")
		if port == "" {
			port = HTTP_PORT
		}
		addr := fmt.Sprintf(":%s", port)
		logger.Logger.Info("Starting HTTP server", "port", port)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start jobs
	logger.Logger.Info("Starting background jobs")

	// Start feed processing job - DISABLED FOR ETHICAL COMPLIANCE
	// Article collection temporarily suspended for ethical reasons
	logger.Logger.Info("Feed processing job disabled for ethical compliance")
	/*
		if err := jobHandler.StartFeedProcessingJob(ctx); err != nil {
			logger.Logger.Error("Failed to start feed processing job", "error", err)
			panic(err)
		}
	*/

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

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop HTTP server
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("Error shutting down HTTP server", "error", err)
	}

	// Stop all jobs
	if err := jobHandler.Stop(); err != nil {
		logger.Logger.Error("Error stopping job handler", "error", err)
	}

	if err := jobScheduler.StopAll(); err != nil {
		logger.Logger.Error("Error stopping job scheduler", "error", err)
	}

	logger.Logger.Info("Pre-processor service stopped")
}
