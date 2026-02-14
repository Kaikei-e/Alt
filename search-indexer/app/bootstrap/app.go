package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"search-indexer/config"
	"search-indexer/consumer"
	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/logger"
	"search-indexer/tokenize"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"

	"github.com/cenkalti/backoff/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// App holds all components of the search-indexer service.
type App struct {
	httpServer    *http.Server
	connectServer *http.Server
	driverClose   func() // closes the article driver (DB pool or noop for API)
	redisConsumer *consumer.Consumer
	otelShutdown  appOtel.ShutdownFunc
}

// Run initializes all components and starts the service.
// It blocks until ctx is cancelled, then performs graceful shutdown.
func Run(ctx context.Context) error {
	// ── OpenTelemetry ──
	otelCfg := appOtel.ConfigFromEnv()
	otelShutdown, err := appOtel.InitProvider(ctx, otelCfg)
	if err != nil {
		fmt.Printf("Failed to initialize OpenTelemetry: %v\n", err)
		otelCfg.Enabled = false
		otelShutdown = func(context.Context) error { return nil }
	}

	// ── Logger ──
	logger.InitWithOTel(otelCfg.Enabled)
	logger.Logger.Info("Starting search-indexer",
		"service", otelCfg.ServiceName,
		"otel_enabled", otelCfg.Enabled,
	)

	// ── Tokenizer ──
	tokenizer, err := tokenize.InitTokenizer()
	if err != nil {
		logger.Logger.Error("Failed to initialize tokenizer", "err", err)
	}

	// ── Load config ──
	appCfg, err := config.Load()
	if err != nil {
		logger.Logger.Error("Failed to load config", "err", err)
		return err
	}

	// ── Drivers (infrastructure layer) ──
	articleDriver, driverClose, err := initArticleDriver(ctx, appCfg)
	if err != nil {
		logger.Logger.Error("Failed to initialize article driver", "err", err)
		return err
	}

	msClient, err := initMeilisearchClient()
	if err != nil {
		logger.Logger.Error("Failed to initialize Meilisearch", "err", err)
		driverClose()
		return err
	}
	searchDriver := driver.NewMeilisearchDriver(msClient, "articles")

	// ── Gateways (anti-corruption layer) ──
	articleRepo := gateway.NewArticleRepositoryGateway(articleDriver)
	searchEngine := gateway.NewSearchEngineGateway(searchDriver)

	if err := searchEngine.EnsureIndex(ctx); err != nil {
		logger.Logger.Error("Failed to ensure search index", "err", err)
		driverClose()
		return err
	}

	// ── Use cases (application layer) ──
	indexUsecase := usecase.NewIndexArticlesUsecase(articleRepo, searchEngine, tokenizer)
	searchByUserUsecase := usecase.NewSearchByUserUsecase(searchEngine)

	// ── Redis Streams Consumer ──
	var redisConsumer *consumer.Consumer
	consumerCfg := consumer.ConfigFromEnv()
	if consumerCfg.Enabled {
		eventHandler := consumer.NewIndexEventHandler(indexUsecase, logger.Logger)
		redisConsumer, err = consumer.NewConsumer(consumerCfg, eventHandler, logger.Logger)
		if err != nil {
			logger.Logger.Error("Failed to create Redis Streams consumer", "err", err)
		} else {
			if err := redisConsumer.Start(ctx); err != nil {
				logger.Logger.Error("Failed to start Redis Streams consumer", "err", err)
			} else {
				logger.Logger.Info("Redis Streams consumer started",
					"stream", consumerCfg.StreamKey,
					"group", consumerCfg.GroupName,
				)
			}
		}
	} else {
		logger.Logger.Info("Redis Streams consumer disabled")
	}

	// ── Batch indexer (polling fallback) ──
	go runIndexLoop(ctx, indexUsecase)

	// ── Servers ──
	app := &App{
		httpServer:    newHTTPServer(searchByUserUsecase, otelCfg),
		connectServer: newConnectServer(searchByUserUsecase),
		driverClose:   driverClose,
		redisConsumer: redisConsumer,
		otelShutdown:  otelShutdown,
	}

	go func() {
		logger.Logger.Info("http listen", "addr", config.HTTPAddr)
		if err := app.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("http", "err", err)
		}
	}()

	go func() {
		logger.Logger.Info("connect-rpc listen", "addr", config.ConnectAddr)
		if err := app.connectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Error("connect-rpc", "err", err)
		}
	}()

	// ── Wait for shutdown signal ──
	<-ctx.Done()
	app.shutdown()
	return nil
}

// shutdown performs graceful shutdown of all components.
func (a *App) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("http shutdown error", "err", err)
	}
	if err := a.connectServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("connect-rpc shutdown error", "err", err)
	}
	if a.redisConsumer != nil {
		a.redisConsumer.Stop()
	}
	if a.driverClose != nil {
		a.driverClose()
	}

	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	if err := a.otelShutdown(otelCtx); err != nil {
		fmt.Printf("Failed to shutdown OpenTelemetry: %v\n", err)
	}
}

// newRetryBackoff creates an exponential backoff policy for index loop retries.
func newRetryBackoff() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 5 * time.Second
	bo.MaxInterval = 5 * time.Minute
	bo.Multiplier = 2
	return bo
}

// runIndexLoop runs the dual-phase indexing loop using clean architecture.
// Phase 1 (Backfill): Index all existing articles from latest to oldest.
// Phase 2 (Incremental): Poll for new articles and sync deletions.
func runIndexLoop(ctx context.Context, indexUsecase *usecase.IndexArticlesUsecase) {
	defer func() {
		if r := recover(); r != nil {
			logger.Logger.Error("index loop panic", "err", r)
		}
	}()

	// Phase 1: Backfill
	var lastCreatedAt *time.Time
	var lastID string
	var incrementalMark *time.Time

	logger.Logger.Info("starting Phase 1: Backfill")

	mark, err := indexUsecase.GetIncrementalMark(ctx)
	if err != nil {
		logger.Logger.Error("failed to get incremental mark", "err", err)
		now := time.Now()
		incrementalMark = &now
		logger.Logger.Info("using current time as incremental mark fallback", "mark", incrementalMark)
	} else if mark == nil {
		now := time.Now()
		incrementalMark = &now
		logger.Logger.Info("no articles found, using current time as incremental mark", "mark", incrementalMark)
	} else {
		incrementalMark = mark
		logger.Logger.Info("incremental mark set", "mark", incrementalMark)
	}

	bo := newRetryBackoff()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		result, err := indexUsecase.ExecuteBackfill(ctx, lastCreatedAt, lastID, config.IndexBatchSize)
		if err != nil {
			recordError(ctx, "backfill")
			delay := bo.NextBackOff()
			logger.Logger.Error("backfill error, retrying", "err", err, "retry_in", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			continue
		}
		bo.Reset()
		recordBatch(ctx, "backfill", result.IndexedCount, result.DeletedCount, time.Since(start))

		if result.IndexedCount == 0 {
			logger.Logger.Info("Phase 1 complete: backfill done")
			break
		}

		logger.Logger.Info("backfill indexed", "count", result.IndexedCount)
		lastCreatedAt = result.LastCreatedAt
		lastID = result.LastID
	}

	// Phase 2: Incremental
	logger.Logger.Info("starting Phase 2: Incremental")

	lastCreatedAt = nil
	lastID = ""
	var lastDeletedAt *time.Time

	bo.Reset()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		result, err := indexUsecase.ExecuteIncremental(ctx, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, config.IndexBatchSize)
		if err != nil {
			recordError(ctx, "incremental")
			delay := bo.NextBackOff()
			logger.Logger.Error("incremental indexing error, retrying", "err", err, "retry_in", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			continue
		}
		bo.Reset()
		recordBatch(ctx, "incremental", result.IndexedCount, result.DeletedCount, time.Since(start))

		if result.IndexedCount > 0 {
			logger.Logger.Info("incremental indexed", "count", result.IndexedCount)
			lastCreatedAt = result.LastCreatedAt
			lastID = result.LastID
		}

		if result.DeletedCount > 0 {
			logger.Logger.Info("deleted from index", "count", result.DeletedCount)
			lastDeletedAt = result.LastDeletedAt
		}

		if result.IndexedCount == 0 && result.DeletedCount == 0 {
			logger.Logger.Info("no new articles or deletions")
		}

		select {
		case <-time.After(config.IndexInterval):
		case <-ctx.Done():
			return
		}
	}
}

// recordBatch records indexing metrics for a completed batch.
func recordBatch(ctx context.Context, phase string, indexed, deleted int, duration time.Duration) {
	m := appOtel.Metrics
	if m == nil {
		return
	}
	attrs := attribute.String("phase", phase)
	if indexed > 0 {
		m.IndexedTotal.Add(ctx, int64(indexed), metric.WithAttributes(attrs))
	}
	if deleted > 0 {
		m.DeletedTotal.Add(ctx, int64(deleted), metric.WithAttributes(attrs))
	}
	m.BatchDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs))
}

// recordError records an error metric.
func recordError(ctx context.Context, operation string) {
	m := appOtel.Metrics
	if m == nil {
		return
	}
	m.ErrorsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", operation)))
}
