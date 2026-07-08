// Package main is the entry point for mq-hub service.
package main

import (
	"context"
	"encoding/json"
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
	cfg, err := config.NewConfig()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load configuration", "error", err)
		os.Exit(1)
	}

	// mq-hub only serves a plaintext listener today; middleware.PeerIdentityMiddleware
	// exists but is not constructed or wired into any handler chain. Log this loudly
	// so "no mTLS enforcement" is an explicit, visible fact rather than something an
	// auditor has to infer from the absence of a wiring call.
	slog.WarnContext(ctx, "peer_identity_disabled",
		"reason", "no mTLS listener configured; PeerIdentityMiddleware is not applied to any handler",
	)

	// Initialize Redis driver with connection pool
	redisDriver, err := driver.NewRedisDriverWithURLAndOptions(cfg.RedisURL, &driver.RedisDriverOptions{
		PoolSize:     cfg.RedisPoolSize,
		StreamMaxLen: cfg.StreamMaxLen,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisDriver.Close()

	// go-redis connects lazily, so a successful constructor call above does
	// not mean Redis is actually reachable. Ping now so a dead Redis fails
	// startup instead of the service reporting healthy with no working backend.
	if err := redisDriver.Ping(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to ping Redis", "error", err)
		os.Exit(1)
	}

	// Initialize gateway
	streamGateway := gateway.NewStreamGateway(redisDriver)

	// Initialize usecases with batch size limit
	publishUsecase := usecase.NewPublishUsecaseWithOptions(streamGateway, &usecase.PublishUsecaseOptions{
		MaxBatchSize: cfg.MaxBatchSize,
	})
	generateTagsUsecase := usecase.NewGenerateTagsUsecase(streamGateway)

	// Initialize handler with tag generation support
	handler := mqhub.NewHandlerWithGenerateTags(publishUsecase, generateTagsUsecase)

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

		// RedisStatus carries err.Error() when unhealthy (see HealthCheck),
		// so it must go through a JSON encoder rather than string
		// concatenation — a quote in the error text would otherwise break
		// the response body and leak internal error detail to the client.
		body := struct {
			Healthy       bool   `json:"healthy"`
			RedisStatus   string `json:"redis_status"`
			UptimeSeconds int64  `json:"uptime_seconds"`
		}{
			Healthy:       health.Healthy,
			UptimeSeconds: health.UptimeSeconds,
		}
		if health.Healthy {
			body.RedisStatus = "connected"
		} else {
			body.RedisStatus = "disconnected"
			slog.WarnContext(r.Context(), "health check failed", "redis_error", health.RedisStatus)
		}

		w.Header().Set("Content-Type", "application/json")
		if health.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.ErrorContext(r.Context(), "failed to encode health response", "error", err)
		}
	})

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Start server with graceful shutdown
	addr := fmt.Sprintf(":%d", cfg.ConnectPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
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
