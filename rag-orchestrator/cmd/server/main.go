package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"rag-orchestrator/internal/adapter/rag_augur"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/logger"
	"rag-orchestrator/internal/usecase"
)

func main() {
	// 1. Load Config
	cfg := config.Load()

	// 2. Initialize Logger
	log := logger.New()

	// 3. Initialize DB
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	dbPool, err := infra.NewPostgresDB(context.Background(), dsn)
	if err != nil {
		log.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// 4. Initialize Adapters
	chunkRepo := repository.NewRagChunkRepository(dbPool)
	docRepo := repository.NewRagDocumentRepository(dbPool)
	embedder := rag_augur.NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel)

	// 5. Initialize Usecases
	// indexUsecase := ... (Phase 4)
	retrieveUsecase := usecase.NewRetrieveContextUsecase(chunkRepo, docRepo, embedder)

	// 6. Initialize Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 7. Initialize Handlers
	handler := rag_http.NewHandler(retrieveUsecase)

	// 8. Register OpenAPI Handlers
	openapi.RegisterHandlers(e, handler)

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

	// 7. Start Server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Info("Starting server", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
}
