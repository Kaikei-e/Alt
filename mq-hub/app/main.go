// Package main is the entry point for mq-hub service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"connectrpc.com/connect"

	"mq-hub/config"
	"mq-hub/connect/v1/mqhub"
	"mq-hub/driver"
	"mq-hub/gateway"
	mqhubv1connect "mq-hub/gen/proto/services/mqhub/v1/mqhubv1connect"
	"mq-hub/usecase"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := config.NewConfig()

	// Initialize Redis driver
	redisDriver, err := driver.NewRedisDriverWithURL(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisDriver.Close()

	// Initialize gateway
	streamGateway := gateway.NewStreamGateway(redisDriver)

	// Initialize usecase
	publishUsecase := usecase.NewPublishUsecase(streamGateway)

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

	// Start server
	addr := fmt.Sprintf(":%d", cfg.ConnectPort)
	slog.Info("starting mq-hub server", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// loggingInterceptor creates a Connect interceptor for logging.
func loggingInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			slog.Info("request received",
				"procedure", req.Spec().Procedure,
				"peer", req.Peer().Addr,
			)

			resp, err := next(ctx, req)

			if err != nil {
				slog.Error("request failed",
					"procedure", req.Spec().Procedure,
					"error", err,
				)
			} else {
				slog.Info("request completed",
					"procedure", req.Spec().Procedure,
				)
			}

			return resp, err
		}
	}
}
