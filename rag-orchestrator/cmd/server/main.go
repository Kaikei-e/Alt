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

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	connectserver "rag-orchestrator/internal/adapter/connect"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/di"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/logger"
	"rag-orchestrator/internal/infra/otel"
)

func main() {
	ctx := context.Background()

	// 1. Load Config
	cfg := config.Load()

	// 2. Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	shutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize OTel provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.ErrorContext(ctx, "failed to shutdown OTel provider", "error", err)
		}
	}()

	// 3. Initialize Logger with OTel support
	log := logger.NewWithOTel(otelCfg.Enabled)
	slog.SetDefault(log)

	// 4. Initialize DB
	dbPool, err := infra.NewPostgresDB(ctx, cfg.DB.DSN(), infra.PoolConfig{
		MaxConns: cfg.DB.MaxConns,
		MinConns: cfg.DB.MinConns,
	})
	if err != nil {
		log.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// 5. Wire all dependencies
	app := di.NewApplicationComponents(cfg, dbPool, log)

	// 6. Start Worker
	app.Worker.Start()
	defer func() {
		log.Info("Stopping worker...")
		app.Worker.Stop()
	}()

	// 7. Initialize Echo
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// 8. Initialize Handlers
	handler := rag_http.NewHandler(
		app.RetrieveUsecase,
		app.AnswerUsecase,
		app.IndexUsecase,
		app.JobRepo,
		app.MorningLetterUsecase,
		rag_http.WithEmbedderOverride(app.EmbedderFactory, app.IndexUsecaseFactory, app.EmbeddingModel, app.EmbedderTimeout),
	)
	openapi.RegisterHandlers(e, handler)
	e.POST("/internal/rag/backfill", handler.Backfill)
	e.POST("/v1/rag/morning-letter", handler.MorningLetter)

	// 9. Health Checks
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		if err := dbPool.Ping(c.Request().Context()); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db down", "error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	// 10. Start Echo Server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Server.Port)
		log.Info("Starting Echo server", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// 11. Start Connect-RPC Server
	connectHandler := connectserver.CreateConnectServer(app.ArticleClient, app.AnswerUsecase, app.RetrieveUsecase, log)
	connectServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Server.ConnectPort),
		Handler:           connectHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("Starting Connect-RPC server", "addr", connectServer.Addr)
		if err := connectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Connect-RPC server error", "error", err)
		}
	}()

	// 12. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := connectServer.Shutdown(ctx); err != nil {
		log.Error("Connect-RPC server shutdown error", "error", err)
	}
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
}
