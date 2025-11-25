package main

import (
	"alt/config"
	"alt/di"
	"alt/driver/alt_db"
	"alt/job"
	"alt/rest"
	"alt/utils/logger"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v4"
)

func main() {
	ctx := context.Background()

	// Load configuration first
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		panic(err)
	}

	log := logger.InitLogger()
	log.Info("Starting server", "port", cfg.Server.Port)

	pool, err := alt_db.InitDBConnectionPool(ctx)
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		panic(err)
	}
	defer pool.Close()

	container := di.NewApplicationComponents(pool)

	// Start background jobs
	go job.HourlyJobRunner(ctx, container.AltDBRepository)
	go job.DailyScrapingPolicyJobRunner(ctx, container.ScrapingDomainUsecase)

	e := echo.New()

	// Server optimization settings
	e.HideBanner = true
	e.HidePort = false

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

	rest.RegisterRoutes(e, container, cfg)

	// Start server in a goroutine
	go func() {
		logger.Logger.Info("Server starting", "port", cfg.Server.Port)
		if err := e.StartServer(server); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("Error starting server", "error", err)
			panic(err)
		}
	}()

	// Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Logger.Info("Shutting down server...")

	// Graceful shutdown with timeout (use server timeout configuration)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.WriteTimeout)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("Error during server shutdown", "error", err)
	}

	logger.Logger.Info("Server stopped")
}
