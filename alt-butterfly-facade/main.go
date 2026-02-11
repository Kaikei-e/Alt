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

	"alt-butterfly-facade/config"
	"alt-butterfly-facade/internal/logger"
	"alt-butterfly-facade/internal/server"
)

func main() {
	// Handle healthcheck subcommand (for Docker healthcheck in distroless image)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	ctx := context.Background()

	// Initialize structured logger with trace context support
	appLogger := logger.Init()

	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.Validate(); err != nil {
		slog.ErrorContext(ctx, "invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "configuration loaded",
		"port", cfg.Port,
		"backend_url", cfg.BackendConnectURL,
		"tts_url", cfg.TTSConnectURL,
		"issuer", cfg.BackendTokenIssuer,
		"audience", cfg.BackendTokenAudience)

	// Load backend token secret
	secret, err := cfg.LoadBackendTokenSecret()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load backend token secret", "error", err)
		os.Exit(1)
	}

	// Create server configuration
	serverCfg := server.Config{
		BackendURL:       cfg.BackendConnectURL,
		Secret:           secret,
		Issuer:           cfg.BackendTokenIssuer,
		Audience:         cfg.BackendTokenAudience,
		RequestTimeout:   cfg.RequestTimeout,
		StreamingTimeout: cfg.StreamingTimeout,
		TTSConnectURL:    cfg.TTSConnectURL,
	}

	// Create HTTP server
	handler := server.NewServer(serverCfg, appLogger)

	address := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         address,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: cfg.StreamingTimeout + 10*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.InfoContext(ctx, "starting alt-butterfly-facade server", "address", address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.InfoContext(ctx, "shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "server exited properly")
}

// runHealthcheck performs a health check against the local server
func runHealthcheck() error {
	port := os.Getenv("BFF_PORT")
	if port == "" {
		port = "9200"
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned status: %d", resp.StatusCode)
	}

	return nil
}
