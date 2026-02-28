package main

import (
	"alt/config"
	connectv2 "alt/connect/v2"
	"alt/di"
	"alt/driver/alt_db"
	"alt/job"
	"alt/middleware"
	"alt/rest"
	"alt/utils/logger"
	altotel "alt/utils/otel"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration first
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		panic(err)
	}

	// Initialize OpenTelemetry
	otelCfg := altotel.ConfigFromEnv()
	otelShutdown, err := altotel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		// Continue without OTel - non-fatal
		otelCfg.Enabled = false
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
		}
	}()

	// Initialize logger with OTel integration
	log := logger.InitLoggerWithOTel(otelCfg.Enabled)
	log.InfoContext(ctx, "Starting server",
		"port", cfg.Server.Port,
		"service", otelCfg.ServiceName,
		"otel_enabled", otelCfg.Enabled,
	)

	pool, err := alt_db.InitDBConnectionPool(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to connect to database", "error", err)
		panic(err)
	}
	defer pool.Close()

	container := di.NewApplicationComponents(pool)

	// Start background jobs via scheduler (context-aware with graceful shutdown)
	scheduler := job.NewJobScheduler()
	scheduler.Add(job.Job{
		Name:     "hourly-feed-collector",
		Interval: 1 * time.Hour,
		Timeout:  30 * time.Minute,
		Fn:       job.CollectFeedsJob(container.AltDBRepository),
	})
	scheduler.Add(job.Job{
		Name:     "daily-scraping-policy",
		Interval: 24 * time.Hour,
		Timeout:  1 * time.Hour,
		Fn:       job.ScrapingPolicyJob(container.ScrapingDomainUsecase),
	})
	scheduler.Add(job.Job{
		Name:     "outbox-worker",
		Interval: 5 * time.Second,
		Timeout:  30 * time.Second,
		Fn:       job.OutboxWorkerJob(container.AltDBRepository, container.RagIntegration),
	})
	scheduler.Start(ctx)

	e := echo.New()

	// Server optimization settings
	e.HideBanner = true
	e.HidePort = false

	// Add OpenTelemetry tracing middleware (creates spans for each request)
	if otelCfg.Enabled {
		e.Use(otelecho.Middleware(otelCfg.ServiceName))
		// Add OTel status middleware to set span status based on HTTP response code
		e.Use(middleware.OTelStatusMiddleware())
	}

	// Custom HTTPErrorHandler to ensure 401 is always returned as 401 (not 404)
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if he, ok := err.(*echo.HTTPError); ok {
			// Keep the original status code (don't modify 401 to 404)
			_ = c.JSON(he.Code, map[string]interface{}{
				"error":  http.StatusText(he.Code),
				"detail": he.Message,
			})
			return
		}
		_ = c.JSON(http.StatusInternalServerError, map[string]interface{}{"error": "internal_error"})
	}

	// Use configuration for server settings
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      e,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	rest.RegisterRoutes(e, container, cfg)

	// Start REST server in a goroutine
	go func() {
		logger.Logger.InfoContext(ctx, "REST server starting", "port", cfg.Server.Port)
		if err := e.StartServer(server); err != nil && err != http.ErrServerClosed {
			logger.Logger.ErrorContext(ctx, "Error starting REST server", "error", err)
			panic(err)
		}
	}()

	// Start Connect-RPC server in a goroutine
	connectPort := 9101
	connectServer := connectv2.CreateConnectServer(container, cfg, log)
	connectHTTPServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", connectPort),
		Handler:           connectServer,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      0, // Unlimited for streaming responses
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		logger.Logger.InfoContext(ctx, "Connect-RPC server starting", "port", connectPort)
		if err := connectHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.ErrorContext(ctx, "Error starting Connect-RPC server", "error", err)
		}
	}()

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Logger.InfoContext(ctx, "Shutting down server...")

	// Cancel context to signal all background jobs to stop
	cancel()

	// Wait for background jobs to finish
	scheduler.Shutdown()
	logger.Logger.Info("Background jobs stopped")

	// Graceful shutdown with timeout (use server timeout configuration)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.WriteTimeout)
	defer shutdownCancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.ErrorContext(shutdownCtx, "Error during REST server shutdown", "error", err)
	}

	if err := connectHTTPServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.ErrorContext(shutdownCtx, "Error during Connect-RPC server shutdown", "error", err)
	}

	logger.Logger.Info("Server stopped")
}
