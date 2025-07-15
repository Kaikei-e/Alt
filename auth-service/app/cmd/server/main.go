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

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"

	"github.com/alt/auth-service/app/config"
	"github.com/alt/auth-service/app/utils/logger"
)

func main() {
	// Load environment variables from .env file if it exists
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("Could not load .env file", "error", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	appLogger, err := logger.New(cfg.LogLevel)
	if err != nil {
		slog.Error("Failed to initialize logger", "error", err)
		os.Exit(1)
	}

	appLogger.Info("Starting Auth Service",
		"version", getVersion(),
		"port", cfg.Port,
		"log_level", cfg.LogLevel)

	// Initialize Echo server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// TODO: Initialize dependencies following Clean Architecture
	// This will be implemented in the next phases:
	// 1. Driver layer (database, kratos client)
	// 2. Gateway layer (adapters)
	// 3. Usecase layer (business logic)
	// 4. REST layer (handlers, middleware)

	// For now, setup basic health endpoint
	setupBasicRoutes(e, appLogger)

	// Start server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Handler:      e,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		appLogger.Info("Server starting", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	appLogger.Info("Server shutting down...")

	// The context is used to inform the server it has 30 seconds to finish
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		appLogger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	appLogger.Info("Server exited")
}

// setupBasicRoutes sets up basic health check routes
func setupBasicRoutes(e *echo.Echo, logger *slog.Logger) {
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"service":   "auth-service",
			"version":   getVersion(),
			"timestamp": time.Now().UTC(),
		})
	})

	e.GET("/health/ready", func(c echo.Context) error {
		// TODO: Add readiness checks (database, kratos connectivity)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": "ready",
		})
	})

	e.GET("/health/live", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": "alive",
		})
	})

	logger.Info("Basic health check routes configured")
}

// getVersion returns the application version
func getVersion() string {
	if version := os.Getenv("VERSION"); version != "" {
		return version
	}
	return "dev"
}