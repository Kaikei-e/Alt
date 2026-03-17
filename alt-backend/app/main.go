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
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

type serverExit struct {
	name string
	err  error
}

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
	otelResult, err := altotel.InitProviderWithMetrics(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		// Continue without OTel - non-fatal
		otelCfg.Enabled = false
		otelResult = &altotel.InitResult{
			Shutdown: func(ctx context.Context) error { return nil },
		}
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := otelResult.Shutdown(shutdownCtx); err != nil {
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

	// Log Go runtime settings for diagnostics
	log.InfoContext(ctx, "Go runtime settings",
		"GOMEMLIMIT", os.Getenv("GOMEMLIMIT"),
		"GOGC", os.Getenv("GOGC"),
		"GOMAXPROCS", runtime.GOMAXPROCS(0),
		"go_version", runtime.Version(),
		"pid", os.Getpid(),
	)

	// Start pprof server if enabled
	var pprofServer *http.Server
	if os.Getenv("PPROF_ENABLED") == "true" {
		runtime.SetMutexProfileFraction(5)
		runtime.SetBlockProfileRate(1)
		pprofServer = &http.Server{Addr: ":6060", Handler: nil}
		go func() { _ = pprofServer.ListenAndServe() }()
		log.InfoContext(ctx, "pprof server started", "port", 6060)
	}

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
	scheduler.Add(job.Job{
		Name:     "ogp-image-warmer",
		Interval: 1 * time.Hour,
		Timeout:  20 * time.Minute,
		Fn:       job.OgpImageWarmerJob(container.AltDBRepository, container.ImageProxyUsecase),
	})
	scheduler.Add(job.Job{
		Name:     "tag-cloud-cache-warmer",
		Interval: 24 * time.Minute,
		Timeout:  2 * time.Minute,
		Fn:       job.TagCloudCacheWarmerJob(container.FetchTagCloudUsecase),
	})
	scheduler.Add(job.Job{
		Name:     "knowledge-projector",
		Interval: 30 * time.Second,
		Timeout:  25 * time.Second,
		Fn: job.KnowledgeProjectorJob(
			container.KnowledgeEventGateway,
			container.KnowledgeProjectionGateway,
			container.KnowledgeProjectionGateway,
			container.KnowledgeHomeGateway,
			container.TodayDigestGateway,
		),
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

	// Register Prometheus metrics endpoint
	if otelResult.MetricsHandler != nil {
		e.GET("/metrics", echo.WrapHandler(otelResult.MetricsHandler))
	}

	rest.RegisterRoutes(e, container, cfg)

	serverExitCh := make(chan serverExit, 2)

	// Start REST server in a goroutine
	go func() {
		logger.Logger.InfoContext(ctx, "REST server starting", "port", cfg.Server.Port)
		err := e.StartServer(server)
		switch {
		case err == nil:
			logger.Logger.InfoContext(ctx, "REST server exited", "reason", "nil_return")
			serverExitCh <- serverExit{name: "rest", err: nil}
		case err == http.ErrServerClosed:
			logger.Logger.InfoContext(ctx, "REST server exited", "reason", "server_closed")
			serverExitCh <- serverExit{name: "rest", err: nil}
		default:
			logger.Logger.ErrorContext(ctx, "REST server exited with error", "error", err)
			serverExitCh <- serverExit{name: "rest", err: err}
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
		err := connectHTTPServer.ListenAndServe()
		switch {
		case err == nil:
			logger.Logger.InfoContext(ctx, "Connect-RPC server exited", "reason", "nil_return")
			serverExitCh <- serverExit{name: "connect_rpc", err: nil}
		case err == http.ErrServerClosed:
			logger.Logger.InfoContext(ctx, "Connect-RPC server exited", "reason", "server_closed")
			serverExitCh <- serverExit{name: "connect_rpc", err: nil}
		default:
			logger.Logger.ErrorContext(ctx, "Connect-RPC server exited with error", "error", err)
			serverExitCh <- serverExit{name: "connect_rpc", err: err}
		}
	}()

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	shutdownReason := "unknown"
	var shutdownSignal os.Signal
	var exitedServer serverExit

	select {
	case sig := <-quit:
		shutdownReason = "signal"
		shutdownSignal = sig
		logger.Logger.WarnContext(ctx, "Shutdown triggered by signal",
			"signal", sig.String(),
		)
	case exit := <-serverExitCh:
		shutdownReason = "server_exit"
		exitedServer = exit
		if exit.err != nil {
			logger.Logger.ErrorContext(ctx, "Shutdown triggered by server exit",
				"server", exit.name,
				"error", exit.err,
			)
		} else {
			logger.Logger.WarnContext(ctx, "Shutdown triggered by server exit",
				"server", exit.name,
				"error", nil,
			)
		}
	}

	logger.Logger.InfoContext(ctx, "Shutting down server...",
		"reason", shutdownReason,
		"signal", signalName(shutdownSignal),
		"server", exitedServer.name,
	)

	// Log final memory stats before shutdown
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	logger.Logger.InfoContext(ctx, "Final memory stats before shutdown",
		"alloc_mib", ms.Alloc/1024/1024,
		"sys_mib", ms.Sys/1024/1024,
		"heap_objects", ms.HeapObjects,
		"num_gc", ms.NumGC,
		"gc_cpu_fraction", ms.GCCPUFraction,
	)

	// Stop pprof server if running
	if pprofServer != nil {
		_ = pprofServer.Close()
	}

	// Cancel context to signal all background jobs to stop
	logger.Logger.InfoContext(ctx, "Cancelling root context for shutdown")
	cancel()

	// Wait for background jobs to finish
	schedulerShutdownStarted := time.Now()
	scheduler.Shutdown()
	logger.Logger.Info("Background jobs stopped",
		"duration_ms", time.Since(schedulerShutdownStarted).Milliseconds(),
	)

	// Graceful shutdown with timeout (use server timeout configuration)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.WriteTimeout)
	defer shutdownCancel()

	logger.Logger.InfoContext(shutdownCtx, "Starting REST server shutdown",
		"timeout", cfg.Server.WriteTimeout.String(),
	)
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.ErrorContext(shutdownCtx, "Error during REST server shutdown", "error", err)
	} else {
		logger.Logger.InfoContext(shutdownCtx, "REST server shutdown completed")
	}

	logger.Logger.InfoContext(shutdownCtx, "Starting Connect-RPC server shutdown",
		"timeout", cfg.Server.WriteTimeout.String(),
	)
	if err := connectHTTPServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.ErrorContext(shutdownCtx, "Error during Connect-RPC server shutdown", "error", err)
	} else {
		logger.Logger.InfoContext(shutdownCtx, "Connect-RPC server shutdown completed")
	}

	logger.Logger.Info("Server stopped",
		"reason", shutdownReason,
		"signal", signalName(shutdownSignal),
		"server", exitedServer.name,
	)
}

func signalName(sig os.Signal) string {
	if sig == nil {
		return ""
	}
	return sig.String()
}
