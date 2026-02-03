// Package main is the entry point for mq-hub service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"mq-hub/config"
	"mq-hub/connect/v1/mqhub"
	"mq-hub/driver"
	"mq-hub/gateway"
	mqhubv1connect "mq-hub/gen/proto/services/mqhub/v1/mqhubv1connect"
	"mq-hub/usecase"
	"mq-hub/utils/logger"
)

func main() {
	ctx := context.Background()

	// Initialize logger with TraceContextHandler for trace_id/span_id propagation
	logger.Init()

	// Load configuration
	cfg := config.NewConfig()

	// Initialize Redis driver with connection pool
	redisDriver, err := driver.NewRedisDriverWithURLAndOptions(cfg.RedisURL, &driver.RedisDriverOptions{
		PoolSize: cfg.RedisPoolSize,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisDriver.Close()

	// Initialize gateway
	streamGateway := gateway.NewStreamGateway(redisDriver)

	// Initialize usecase with batch size limit
	publishUsecase := usecase.NewPublishUsecaseWithOptions(streamGateway, &usecase.PublishUsecaseOptions{
		MaxBatchSize: cfg.MaxBatchSize,
	})

	// Initialize handler
	handler := mqhub.NewHandler(publishUsecase)

	// Create HTTP mux
	mux := http.NewServeMux()

	// Register Connect-RPC handler
	path, h := mqhubv1connect.NewMQHubServiceHandler(handler,
		connect.WithInterceptors(loggingInterceptor()),
	)
	mux.Handle(path, h)

	// Health check endpoint for non-RPC clients
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health := publishUsecase.HealthCheck(r.Context())
		if health.Healthy {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"healthy":true,"redis_status":"%s","uptime_seconds":%d}`, health.RedisStatus, health.UptimeSeconds)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"healthy":false,"redis_status":"%s","uptime_seconds":%d}`, health.RedisStatus, health.UptimeSeconds)
		}
	})

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Start server with graceful shutdown
	addr := fmt.Sprintf(":%d", cfg.ConnectPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Channel to signal server startup errors
	serverErr := make(chan error, 1)

	go func() {
		slog.InfoContext(ctx, "starting mq-hub server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGTERM/SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		slog.InfoContext(ctx, "received shutdown signal", "signal", sig.String())
	case err := <-serverErr:
		slog.ErrorContext(ctx, "server failed to start", "error", err)
		os.Exit(1)
	}

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.InfoContext(ctx, "shutting down server gracefully")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "server shutdown complete")
}

// loggingInterceptor creates a Connect interceptor for logging.
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			slog.InfoContext(ctx, "request received",
				"procedure", req.Spec().Procedure,
				"peer", req.Peer().Addr,
			)

			resp, err := next(ctx, req)

			if err != nil {
				slog.ErrorContext(ctx, "request failed",
					"procedure", req.Spec().Procedure,
					"error", err,
				)
			} else {
				slog.InfoContext(ctx, "request completed",
					"procedure", req.Spec().Procedure,
				)
			}

			return resp, err
		}
	}
}
