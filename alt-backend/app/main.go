package main

import (
	"alt/di"
	"alt/driver/alt_db"
	"alt/job"
	"alt/rest"
	"alt/utils/logger"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
)

func main() {
	ctx := context.Background()

	log := logger.InitLogger()
	log.Info("Starting server")

	db, err := alt_db.InitDBConnection(ctx)
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		panic(err)
	}
	defer db.Close(context.Background())

	container := di.NewApplicationComponents(db)

	// Start background job
	go job.HourlyJobRunner(ctx, container.AltDBRepository)

	e := echo.New()

	// Server optimization settings
	e.HideBanner = true
	e.HidePort = false

	// Optimize server configuration
	server := &http.Server{
		Addr:         ":9000",
		Handler:      e,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	rest.RegisterRoutes(e, container)

	// Start server in a goroutine
	go func() {
		logger.Logger.Info("Server starting on port 9000")
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

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("Error during server shutdown", "error", err)
	}

	logger.Logger.Info("Server stopped")
}
