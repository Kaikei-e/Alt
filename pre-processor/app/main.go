package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pre-processor/config"
	connectv2 "pre-processor/connect/v2"
	"pre-processor/consumer"
	"pre-processor/driver"
	"pre-processor/handler"
	appmiddleware "pre-processor/middleware"
	"pre-processor/repository"
	"pre-processor/service"
	logger "pre-processor/utils/logger"
	"pre-processor/utils/otel"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

const (
	BATCH_SIZE       = 40
	NEWS_CREATOR_URL = "http://news-creator:11434"
	HTTP_PORT        = "9200" // Default HTTP port for API
	CONNECT_PORT     = "9202" // Default Connect-RPC port (9201 is used by auth-token-manager)
)

func performHealthCheck() {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = HTTP_PORT
	}
	rawURL := fmt.Sprintf("http://localhost:%s/api/v1/health", port)

	logger.Logger.Info("Performing health check", "url", rawURL)

	urlParsed, err := url.Parse(rawURL)
	if err != nil {
		logger.Logger.Error("Failed to parse URL", "error", err)
		panic(err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(urlParsed.String())
	if err != nil {
		os.Exit(1)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Logger.Warn("health check: failed to close response body", "error", cerr, "url", rawURL)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}

// getEnvOrDefault returns the value of an environment variable or a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Support both --health-check flag and healthcheck subcommand
	if len(os.Args) > 1 && (os.Args[1] == "--health-check" || os.Args[1] == "healthcheck") {
		performHealthCheck()
		return
	}

	ctx := context.Background()

	// Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	otelShutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		// Continue without OTel - non-fatal
		otelCfg.Enabled = false
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
		}
	}()

	// Load logger configuration from environment
	loggerConfig := logger.LoadLoggerConfigFromEnv()

	// Initialize logger with OTel support
	contextLogger := logger.NewContextLoggerWithOTel(loggerConfig, otelCfg.Enabled)

	// Use context logger as primary logger
	logger.Logger = contextLogger.WithContext(ctx)
	logger.Logger.Info("Starting pre-processor service",
		"log_level", loggerConfig.Level,
		"log_format", loggerConfig.Format,
		"otel_enabled", otelCfg.Enabled,
		"service", otelCfg.ServiceName)

	// Initialize database

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
	jobRepo := repository.NewSummarizeJobRepository(dbPool, logger.Logger)

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

	// Initialize article sync service
	articleSyncService := service.NewArticleSyncService(
		articleRepo,
		apiRepo,
		logger.Logger,
	)

	// Initialize summarize queue worker
	summarizeQueueWorker := service.NewSummarizeQueueWorker(
		jobRepo,
		articleRepo,
		apiRepo,
		summaryRepo,
		logger.Logger,
		BATCH_SIZE,
	)

	// Initialize health metrics collector
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	// Initialize Redis Streams consumer (if enabled)
	consumerCfg := consumer.Config{
		RedisURL:      getEnvOrDefault("REDIS_STREAMS_URL", "redis://redis-streams:6379"),
		GroupName:     getEnvOrDefault("CONSUMER_GROUP", "pre-processor-group"),
		ConsumerName:  getEnvOrDefault("CONSUMER_NAME", "pre-processor-1"),
		StreamKey:     "alt:events:articles",
		BatchSize:     10,
		BlockTimeout:  5 * time.Second,
		ClaimIdleTime: 30 * time.Second,
		Enabled:       getEnvOrDefault("CONSUMER_ENABLED", "false") == "true",
	}

	summarizeServiceAdapter := NewSummarizeServiceAdapter(jobRepo, articleRepo, logger.Logger)
	eventHandler := consumer.NewPreProcessorEventHandler(summarizeServiceAdapter, logger.Logger)
	redisConsumer, err := consumer.NewConsumer(consumerCfg, eventHandler, logger.Logger)
	if err != nil {
		logger.Logger.Error("Failed to create Redis Streams consumer", "error", err)
	} else {
		if err := redisConsumer.Start(ctx); err != nil {
			logger.Logger.Error("Failed to start Redis Streams consumer", "error", err)
		} else {
			logger.Logger.Info("Redis Streams consumer started",
				"stream", consumerCfg.StreamKey,
				"group", consumerCfg.GroupName,
				"enabled", consumerCfg.Enabled)
		}
		defer redisConsumer.Stop()
	}

	// Initialize handlers
	jobHandler := handler.NewJobHandler(
		feedProcessorService,
		articleSummarizerService,
		qualityCheckerService,
		articleSyncService,
		healthCheckerService,
		summarizeQueueWorker,
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
		articleRepo,
		jobRepo,
		logger.Logger,
	)

	jobScheduler := handler.NewJobScheduler(logger.Logger)

	// Initialize Echo HTTP server
	e := echo.New()
	e.HideBanner = true

	// Custom error handler for consistent error responses
	e.HTTPErrorHandler = appmiddleware.CustomHTTPErrorHandler(logger.Logger)

	// Add OpenTelemetry tracing middleware (creates spans for each request)
	if otelCfg.Enabled {
		e.Use(otelecho.Middleware(otelCfg.ServiceName))
		// Add OTel status middleware to set span status based on HTTP response code
		e.Use(appmiddleware.OTelStatusMiddleware())
	}

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:  true,
		LogURI:     true,
		LogStatus:  true,
		LogLatency: true,
		LogError:   true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()
			logger.Logger.InfoContext(ctx, "HTTP request completed",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"latency", v.Latency,
				"error", v.Error)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API routes
	api := e.Group("/api/v1")
	api.POST("/summarize", summarizeHandler.HandleSummarize)                     // Legacy synchronous endpoint
	api.POST("/summarize/stream", summarizeHandler.HandleStreamSummarize)        // Streaming endpoint
	api.POST("/summarize/queue", summarizeHandler.HandleSummarizeQueue)          // New async queue endpoint
	api.GET("/summarize/status/:job_id", summarizeHandler.HandleSummarizeStatus) // Job status endpoint
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

	// Start Connect-RPC server in goroutine
	connectServer := connectv2.CreateConnectServer(apiRepo, summaryRepo, articleRepo, jobRepo, logger.Logger)
	go func() {
		connectPort := os.Getenv("CONNECT_PORT")
		if connectPort == "" {
			connectPort = CONNECT_PORT
		}
		addr := fmt.Sprintf(":%s", connectPort)
		logger.Logger.Info("Starting Connect-RPC server", "port", connectPort)
		server := &http.Server{
			Addr:         addr,
			Handler:      connectServer,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("Connect-RPC server error", "error", err)
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

	// Start article sync job
	if err := jobHandler.StartArticleSyncJob(ctx); err != nil {
		logger.Logger.Error("Failed to start article sync job", "error", err)
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

	// Start summarize queue worker
	if err := jobHandler.StartSummarizeQueueWorker(ctx); err != nil {
		logger.Logger.Error("Failed to start summarize queue worker", "error", err)
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
