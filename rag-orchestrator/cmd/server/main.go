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
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/logger"
	"rag-orchestrator/internal/usecase"
	"rag-orchestrator/internal/worker"
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
	jobRepo := repository.NewRagJobRepository(dbPool)
	txManager := repository.NewPostgresTransactionManager(dbPool)
	embedder := rag_augur.NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel)

	// 5. Initialize Usecases
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	indexUsecase := usecase.NewIndexArticleUsecase(
		docRepo,
		chunkRepo,
		txManager,
		hasher,
		chunker,
		embedder,
	)

	retrieveUsecase := usecase.NewRetrieveContextUsecase(chunkRepo, docRepo, embedder)
	generator := rag_augur.NewOllamaGenerator(cfg.KnowledgeAugurURL, cfg.KnowledgeAugurModel)
	promptBuilder := usecase.NewXMLPromptBuilder("Answer in Japanese.")
	answerUsecase := usecase.NewAnswerWithRAGUsecase(
		retrieveUsecase,
		promptBuilder,
		generator,
		usecase.NewOutputValidator(),
		cfg.AnswerMaxChunks,
		cfg.AnswerMaxTokens,
		cfg.PromptVersion,
		cfg.DefaultLocale,
	)

	// 6. Initialize & Start Worker
	jobWorker := worker.NewJobWorker(jobRepo, indexUsecase, log)
	jobWorker.Start()
	// Ensure worker stops on shutdown
	defer func() {
		log.Info("Stopping worker...")
		jobWorker.Stop()
	}()

	// 7. Initialize Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 8. Initialize Handlers
	handler := rag_http.NewHandler(retrieveUsecase, answerUsecase, jobRepo)

	// 9. Register OpenAPI Handlers
	openapi.RegisterHandlers(e, handler)

	// 10. Manual Registration for Backfill
	e.POST("/internal/rag/backfill", handler.Backfill)

	// 11. Health Checks
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		if err := dbPool.Ping(c.Request().Context()); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db down", "error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	// 12. Start Server
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Info("Starting server", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// 13. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
}
