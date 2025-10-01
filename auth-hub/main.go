package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"auth-hub/cache"
	"auth-hub/client"
	"auth-hub/config"
	"auth-hub/handler"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		"kratos_url", cfg.KratosURL,
		"port", cfg.Port,
		"cache_ttl", cfg.CacheTTL)

	// Initialize dependencies
	sessionCache := cache.NewSessionCache(cfg.CacheTTL)
	slog.Info("session cache initialized", "ttl", cfg.CacheTTL)

	kratosClient := client.NewKratosClient(cfg.KratosURL, 5*time.Second)
	slog.Info("kratos client initialized", "base_url", cfg.KratosURL)

	// Initialize handlers
	validateHandler := handler.NewValidateHandler(kratosClient, sessionCache)
	healthHandler := handler.NewHealthHandler()

	// Setup Echo server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error == nil {
				slog.Info("request completed",
					"method", v.Method,
					"uri", v.URI,
					"status", v.Status,
					"latency_ms", v.Latency.Milliseconds())
			} else {
				slog.Error("request failed",
					"method", v.Method,
					"uri", v.URI,
					"status", v.Status,
					"latency_ms", v.Latency.Milliseconds(),
					"error", v.Error.Error())
			}
			return nil
		},
	}))

	e.Use(middleware.Recover())

	// Register routes
	e.GET("/validate", validateHandler.Handle)
	e.GET("/health", healthHandler.Handle)

	// Start server
	address := fmt.Sprintf(":%s", cfg.Port)
	slog.Info("starting auth-hub server", "address", address)

	if err := e.Start(address); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
