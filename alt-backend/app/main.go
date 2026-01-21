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
	ctx := context.Background()

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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
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

	// Start background jobs
	go job.HourlyJobRunner(ctx, container.AltDBRepository)
	go job.DailyScrapingPolicyJobRunner(ctx, container.ScrapingDomainUsecase)
	go job.OutboxWorkerRunner(ctx, container.AltDBRepository, container.RagIntegration)

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
	go func() {
		logger.Logger.InfoContext(ctx, "Connect-RPC server starting", "port", connectPort)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", connectPort), connectServer); err != nil && err != http.ErrServerClosed {
			logger.Logger.ErrorContext(ctx, "Error starting Connect-RPC server", "error", err)
		}
	}()

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Logger.InfoContext(ctx, "Shutting down server...")

	// Graceful shutdown with timeout (use server timeout configuration)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.WriteTimeout)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.ErrorContext(shutdownCtx, "Error during server shutdown", "error", err)
	}

	logger.Logger.InfoContext(ctx, "Server stopped")
}
