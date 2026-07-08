package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	logger "pre-processor/utils/logger"
	"pre-processor/utils/otel"
)

// Run is the main application entry point. It initializes all dependencies,
// starts servers and background jobs, then waits for a shutdown signal.
func Run(ctx context.Context) error {
	// Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	otelShutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		otelCfg.Enabled = false
	}
	otelShutdown = resolveOtelShutdown(otelShutdown)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
		}
	}()

	// Initialize logger
	loggerConfig := logger.LoadLoggerConfigFromEnv()
	contextLogger := logger.NewContextLoggerWithOTel(loggerConfig, otelCfg.Enabled)
	log := contextLogger.WithContext(ctx)
	logger.Logger = log

	log.Info("Starting pre-processor service",
		"log_level", loggerConfig.Level,
		"log_format", loggerConfig.Format,
		"otel_enabled", otelCfg.Enabled,
		"service", otelCfg.ServiceName)

	// Build all dependencies
	deps, cleanup, err := BuildDependencies(ctx, log, otelCfg.Enabled)
	if err != nil {
		return fmt.Errorf("failed to build dependencies: %w", err)
	}
	defer cleanup()
	defer func() {
		if deps.RedisConsumer != nil {
			deps.RedisConsumer.Stop()
		}
	}()

	// Start servers. errCh is buffered so a failing listener never blocks on
	// send even if multiple servers fail before waitForShutdown starts reading.
	errCh := make(chan error, 3)
	httpServer := NewHTTPServer(deps, otelCfg.Enabled, otelCfg.ServiceName)
	StartHTTPServer(httpServer, log, errCh)
	connectServers := StartConnectServer(deps, errCh)

	// Start background jobs
	if err := startJobs(ctx, deps, log); err != nil {
		return fmt.Errorf("failed to start jobs: %w", err)
	}

	// Wait for shutdown signal
	log.Info("Pre-processor service started successfully")
	return waitForShutdown(httpServer, connectServers, deps, log, errCh)
}

// resolveOtelShutdown returns fn unchanged when it is non-nil. otel.InitProvider
// returns (nil, err) on initialization failure — calling a nil ShutdownFunc from
// the deferred shutdown in Run would panic, so callers must fall back to a no-op.
func resolveOtelShutdown(fn otel.ShutdownFunc) otel.ShutdownFunc {
	if fn != nil {
		return fn
	}
	return func(context.Context) error { return nil }
}

func startJobs(ctx context.Context, deps *Dependencies, log *slog.Logger) error {
	log.Info("Starting background jobs")

	if err := deps.JobHandler.StartArticleSyncJob(ctx); err != nil {
		return fmt.Errorf("failed to start article sync job: %w", err)
	}
	if err := deps.JobHandler.StartBackfillJob(ctx); err != nil {
		return fmt.Errorf("failed to start backfill job: %w", err)
	}
	if err := deps.JobHandler.StartSummarizationJob(ctx); err != nil {
		return fmt.Errorf("failed to start summarization job: %w", err)
	}
	if err := deps.JobHandler.StartQualityCheckJob(ctx); err != nil {
		return fmt.Errorf("failed to start quality check job: %w", err)
	}
	if err := deps.JobHandler.StartSummarizeQueueWorker(ctx); err != nil {
		return fmt.Errorf("failed to start summarize queue worker: %w", err)
	}

	// Non-fatal dependency health check
	if err := deps.HealthHandler.CheckDependencies(ctx); err != nil {
		log.Warn("Some dependencies are not healthy", "error", err)
	}

	return nil
}

// waitForShutdown blocks until either a termination signal arrives or one of
// the servers reports a fatal ListenAndServe failure on errCh, then shuts
// down every listener (HTTP, Connect-RPC, and the mTLS listener when
// enabled) and the background job handler. It returns a non-nil error when
// the shutdown was triggered by a server failure so main() exits non-zero
// instead of limping along with a dead listener.
func waitForShutdown(httpServer interface{ Shutdown(context.Context) error }, connectServers *ConnectServers, deps *Dependencies, log *slog.Logger, errCh <-chan error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var shutdownErr error
	select {
	case sig := <-quit:
		log.Info("Shutting down pre-processor service", "signal", sig.String())
	case err := <-errCh:
		log.Error("Shutting down pre-processor service due to server failure", "error", err)
		shutdownErr = err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("Error shutting down HTTP server", "error", err)
	}

	if connectServers != nil {
		if connectServers.Server != nil {
			if err := connectServers.Server.Shutdown(shutdownCtx); err != nil {
				log.Error("Error shutting down Connect-RPC server", "error", err)
			}
		}
		if connectServers.MTLSServer != nil {
			if err := connectServers.MTLSServer.Shutdown(shutdownCtx); err != nil {
				log.Error("Error shutting down mTLS listener", "error", err)
			}
		}
	}

	if err := deps.JobHandler.Stop(); err != nil {
		log.Error("Error stopping job handler", "error", err)
	}

	log.Info("Pre-processor service stopped")
	return shutdownErr
}
