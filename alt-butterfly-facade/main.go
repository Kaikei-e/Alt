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

	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := config.NewConfig()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		"port", cfg.Port,
		"backend_url", cfg.BackendConnectURL,
		"issuer", cfg.BackendTokenIssuer,
		"audience", cfg.BackendTokenAudience)

	// Load backend token secret
	secret, err := cfg.LoadBackendTokenSecret()
	if err != nil {
		slog.Error("failed to load backend token secret", "error", err)
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
	}

	// Create HTTP server
	handler := server.NewServer(serverCfg, logger)

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
		slog.Info("starting alt-butterfly-facade server", "address", address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited properly")
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
