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

	"auth-hub/cache"
	"auth-hub/client"
	"auth-hub/config"
	"auth-hub/handler"
	"auth-hub/utils/logger"
	"auth-hub/utils/otel"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
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

	// Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	otelShutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		otelCfg.Enabled = false
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
		}
	}()

	// Initialize structured logger with OTel support
	logger.Init(otelCfg.Enabled)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.ErrorContext(ctx, "failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "configuration loaded",
		"kratos_url", cfg.KratosURL,
		"port", cfg.Port,
		"cache_ttl", cfg.CacheTTL)

	// Initialize dependencies
	sessionCache := cache.NewSessionCache(cfg.CacheTTL)
	slog.InfoContext(ctx, "session cache initialized", "ttl", cfg.CacheTTL)

	kratosClient := client.NewKratosClientWithAdmin(cfg.KratosURL, cfg.KratosAdminURL, 5*time.Second)
	slog.InfoContext(ctx, "kratos client initialized",
		"base_url", cfg.KratosURL,
		"admin_url", cfg.KratosAdminURL)

	// Initialize handlers
	validateHandler := handler.NewValidateHandler(kratosClient, sessionCache)
	sessionHandler := handler.NewSessionHandler(kratosClient, sessionCache, cfg.AuthSharedSecret, cfg)
	csrfHandler := handler.NewCSRFHandler(kratosClient, cfg)
	healthHandler := handler.NewHealthHandler()
	internalHandler := handler.NewInternalHandler(kratosClient)

	// Setup Echo server
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Add OpenTelemetry tracing middleware
	if otelCfg.Enabled {
		e.Use(otelecho.Middleware(otelCfg.ServiceName))
	}

	// Middleware
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			ctx := c.Request().Context()
			if v.Error == nil {
				slog.InfoContext(ctx, "request completed",
					"method", v.Method,
					"uri", v.URI,
					"status", v.Status,
					"latency_ms", v.Latency.Milliseconds())
			} else {
				slog.ErrorContext(ctx, "request failed",
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
	e.GET("/session", sessionHandler.Handle)
	e.POST("/csrf", csrfHandler.Handle)
	e.GET("/health", healthHandler.Handle)

	// Internal routes for service-to-service communication
	e.GET("/internal/system-user", internalHandler.HandleSystemUser)

	// Start server
	address := fmt.Sprintf(":%s", cfg.Port)

	// Start server in a goroutine
	go func() {
		slog.InfoContext(ctx, "starting auth-hub server", "address", address)
		if err := e.Start(address); err != nil && err != http.ErrServerClosed {
			slog.ErrorContext(ctx, "server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.InfoContext(ctx, "shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		slog.ErrorContext(ctx, "server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.InfoContext(ctx, "server exited properly")
}

// runHealthcheck performs a health check against the local server
func runHealthcheck() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
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
