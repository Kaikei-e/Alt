package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"search-indexer/config"
	"search-indexer/consumer"
	"search-indexer/driver"
	"search-indexer/driver/recap_api"
	"search-indexer/gateway"
	"search-indexer/logger"
	"search-indexer/tlsutil"
	"search-indexer/tokenize"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"

	"github.com/cenkalti/backoff/v6"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// App holds all components of the search-indexer service.
type App struct {
	httpServer    *http.Server
	connectServer *http.Server
	mtlsServer    *http.Server
	redisConsumer *consumer.Consumer
	eventHandler  *consumer.IndexEventHandler
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
	// Fail-fast: a nil tokenizer silently injected into the usecase used to
	// panic on the first Japanese tag deep inside kagome, which
	// runIndexLoop's recover caught and (before this fix) returned from --
	// permanently halting indexing while health stayed green (CLAUDE.md
	// rule 8). Treat init failure as a startup error instead.
	tokenizer, err := tokenize.InitTokenizer()
	if err != nil {
		logger.Logger.Error("Failed to initialize tokenizer", "err", err)
		return fmt.Errorf("tokenizer init: %w", err)
	}

	// ── Load config ──
	appCfg, err := config.Load()
	if err != nil {
		logger.Logger.Error("Failed to load config", "err", err)
		return err
	}

	// ── Drivers (infrastructure layer) ──
	articleDriver, err := initArticleDriver(appCfg)
	if err != nil {
		logger.Logger.Error("Failed to initialize article driver", "err", err)
		return err
	}

	msClient, searchOnlyClient, err := initMeilisearchClients()
	if err != nil {
		logger.Logger.Error("Failed to initialize Meilisearch", "err", err)
		return err
	}
	searchDriver := driver.NewMeilisearchDriverWithClients(msClient, searchOnlyClient, "articles").
		WithHybrid(&driver.HybridConfig{
			Embedder:      config.MeiliHybridEmbedder,
			SemanticRatio: config.MeiliHybridSemanticRatio,
		}).
		WithCache(config.MeiliSearchCacheSize, config.MeiliSearchCacheTTL)

	// ── Gateways (anti-corruption layer) ──
	articleRepo := gateway.NewArticleRepositoryGateway(articleDriver)
	searchEngine := gateway.NewSearchEngineGateway(searchDriver)

	if err := searchEngine.EnsureIndex(ctx); err != nil {
		logger.Logger.Error("Failed to ensure search index", "err", err)
		return err
	}

	// Issue a probe Search so Meilisearch loads the qwen3 embedding model into
	// Ollama's resident set before real traffic arrives. Without this the first
	// user-facing search pays the embedder cold-start cost (~1.1s observed).
	// Goroutine so a stalled embedder cannot delay service start.
	go warmupSearchEngine(ctx, searchEngine)

	// ── Recap drivers & gateways ──
	var indexRecapsUsecase *usecase.IndexRecapsUsecase
	var searchRecapsUsecase *usecase.SearchRecapsUsecase

	if config.RecapWorkerURL != "" {
		recapClient := recap_api.NewClient(config.RecapWorkerURL)
		recapDriver := driver.NewMeilisearchRecapDriver(msClient)
		recapRepo := gateway.NewRecapRepositoryGateway(recapClient)
		recapSearchEngine := gateway.NewRecapSearchEngineGateway(recapDriver)

		if err := recapSearchEngine.EnsureRecapIndex(ctx); err != nil {
			logger.Logger.Error("Failed to ensure recap search index", "err", err)
			// Non-fatal: continue without recap indexing
		} else {
			indexRecapsUsecase = usecase.NewIndexRecapsUsecase(recapRepo, recapSearchEngine)
			searchRecapsUsecase = usecase.NewSearchRecapsUsecase(recapSearchEngine)
			logger.Logger.Info("Recap indexing enabled", "recap_worker_url", config.RecapWorkerURL)
		}
	} else {
		logger.Logger.Info("Recap indexing disabled (RECAP_WORKER_URL not set)")
	}

	// ── Use cases (application layer) ──
	indexUsecase := usecase.NewIndexArticlesUsecase(articleRepo, searchEngine, tokenizer)
	searchByUserUsecase := usecase.NewSearchByUserUsecase(searchEngine)
	searchArticlesUsecase := usecase.NewSearchArticlesUsecase(searchEngine)

	// ── Redis Streams Consumer ──
	var redisConsumer *consumer.Consumer
	var eventHandler *consumer.IndexEventHandler
	consumerCfg := consumer.ConfigFromEnv()
	if consumerCfg.Enabled {
		eventHandler = consumer.NewIndexEventHandler(indexUsecase, logger.Logger)
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

	// ── Recap indexer ──
	if indexRecapsUsecase != nil {
		go runRecapIndexLoop(ctx, indexRecapsUsecase)
	}

	// ── Servers ──
	app := &App{
		httpServer:    newHTTPServer(searchByUserUsecase, searchArticlesUsecase, otelCfg, appCfg.RateLimit),
		connectServer: newConnectServer(searchByUserUsecase, searchRecapsUsecase, appCfg.RateLimit),
		redisConsumer: redisConsumer,
		eventHandler:  eventHandler,
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

	if os.Getenv("MTLS_LISTEN") == "true" {
		mtlsPort := os.Getenv("MTLS_PORT")
		if mtlsPort == "" {
			mtlsPort = "9443"
		}
		tlsCfg, tlsErr := tlsutil.LoadServerConfig(
			os.Getenv("MTLS_CERT_FILE"),
			os.Getenv("MTLS_KEY_FILE"),
			os.Getenv("MTLS_CA_FILE"),
			tlsutil.OptionsFromEnv()...,
		)
		if tlsErr != nil {
			return fmt.Errorf("mTLS listener config (fail-closed): %w", tlsErr)
		}
		{
			// :9443 serves REST + Connect-RPC with peer-identity
			// enforcement. Plain :9300 / :9301 remain during the migration
			// window for callers that have not yet switched to mTLS.
			mtlsHandler := newMTLSMuxHandler(
				searchByUserUsecase,
				searchArticlesUsecase,
				searchRecapsUsecase,
				app.connectServer.Handler,
				otelCfg,
				appCfg.RateLimit,
			)
			app.mtlsServer = tlsutil.NewMTLSHTTPServer(":"+mtlsPort, tlsCfg, mtlsHandler)
			go func() {
				logger.Logger.Info("mtls listen (REST + Connect-RPC, peer-identity gated)", "port", mtlsPort)
				if err := app.mtlsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					logger.Logger.Error("mtls", "err", err)
				}
			}()
		}
	}

	// ── Wait for shutdown signal ──
	<-ctx.Done()
	app.shutdown()
	return nil
}

// shutdown performs graceful shutdown of all components.
//
// The Redis consumer's intake (XREADGROUP/XAUTOCLAIM loops) is halted
// before the event handler flushes, and the handler flushes (and ACKs)
// before the consumer's Redis client is closed. Reversing this order was
// the HIGH finding here: eventHandler.Stop() was never called at all, so up
// to ~2s of buffered, already-ACKed events were silently dropped on every
// restart. See .claude/rules/event-stream-consumer.md shutdown ordering.
func (a *App) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("http shutdown error", "err", err)
	}
	if err := a.connectServer.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("connect-rpc shutdown error", "err", err)
	}
	if a.mtlsServer != nil {
		if err := a.mtlsServer.Shutdown(shutdownCtx); err != nil {
			logger.Logger.Error("mtls shutdown error", "err", err)
		}
	}

	if a.redisConsumer != nil {
		a.redisConsumer.StopIntake()
	}
	if a.eventHandler != nil {
		a.eventHandler.Stop()
	}
	if a.redisConsumer != nil {
		a.redisConsumer.Close()
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

// indexLoopRestartDelay bounds how quickly runIndexLoop restarts after an
// unrecovered panic, so a persistent bug doesn't spin the goroutine in a
// tight crash loop.
const indexLoopRestartDelay = 5 * time.Second

// safeExecuteBackfill guards ExecuteBackfill against panics -- e.g. a nil
// tokenizer reached through registerBatchSynonyms when a batch contains a
// Japanese tag -- converting them into a plain error. This lets the
// existing backoff-and-retry loop in runIndexLoop treat "panicked" exactly
// like any other transient failure instead of the panic unwinding the
// whole goroutine and permanently halting indexing while health stays
// green (CLAUDE.md rule 8).
func safeExecuteBackfill(ctx context.Context, uc *usecase.IndexArticlesUsecase, lastCreatedAt *time.Time, lastID string, batchSize int) (result *usecase.IndexResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(ctx, "backfill panic", r)
			err = fmt.Errorf("recovered panic in backfill: %v", r)
		}
	}()
	return uc.ExecuteBackfill(ctx, lastCreatedAt, lastID, batchSize)
}

// safeExecuteIncremental mirrors safeExecuteBackfill for Phase 2.
func safeExecuteIncremental(ctx context.Context, uc *usecase.IndexArticlesUsecase, incrementalMark, lastCreatedAt *time.Time, lastID string, lastDeletedAt *time.Time, batchSize int) (result *usecase.IndexResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(ctx, "incremental panic", r)
			err = fmt.Errorf("recovered panic in incremental indexing: %v", r)
		}
	}()
	return uc.ExecuteIncremental(ctx, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, batchSize)
}

// runIndexLoop runs the dual-phase indexing loop using clean architecture.
// Phase 1 (Backfill): Index all existing articles from latest to oldest.
// Phase 2 (Incremental): Poll for new articles and sync deletions.
//
// Per-batch panics inside either phase are recovered close to the source by
// safeExecuteBackfill/safeExecuteIncremental and folded into the existing
// backoff-and-retry path above, which keeps lastCreatedAt/lastID progress
// intact. The recover below is a last-resort safety net for anything else:
// it restarts the loop instead of letting the goroutine exit, because the
// previous behavior -- log and return -- silently and permanently halted
// indexing while the service kept reporting healthy.
func runIndexLoop(ctx context.Context, indexUsecase *usecase.IndexArticlesUsecase) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(ctx, "index loop panic, restarting", r)
			select {
			case <-ctx.Done():
				return
			case <-time.After(indexLoopRestartDelay):
			}
			go runIndexLoop(ctx, indexUsecase)
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
		result, err := safeExecuteBackfill(ctx, indexUsecase, lastCreatedAt, lastID, config.IndexBatchSize)
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
		result, err := safeExecuteIncremental(ctx, indexUsecase, incrementalMark, lastCreatedAt, lastID, lastDeletedAt, config.IndexBatchSize)
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

// runRecapIndexLoop runs dual-phase indexing for recap genres.
// Phase 1: Backfill all existing recap genres.
// Phase 2: Incremental polling for new recap genres.
func runRecapIndexLoop(ctx context.Context, indexRecapsUsecase *usecase.IndexRecapsUsecase) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(ctx, "recap index loop panic", r)
		}
	}()

	// Phase 1: Backfill
	logger.Logger.Info("starting Recap Phase 1: Backfill")

	bo := newRetryBackoff()
	var lastSince string
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		result, err := indexRecapsUsecase.ExecuteBackfill(ctx, lastSince, config.RecapIndexBatchSize)
		if err != nil {
			recordError(ctx, "recap_backfill")
			delay := bo.NextBackOff()
			logger.Logger.Error("recap backfill error, retrying", "err", err, "retry_in", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			continue
		}
		bo.Reset()
		recordBatch(ctx, "recap_backfill", result.IndexedCount, 0, time.Since(start))

		if result.LastSince != "" {
			lastSince = result.LastSince
		}

		if !result.HasMore {
			logger.Logger.Info("Recap Phase 1 complete: backfill done", "indexed_last_batch", result.IndexedCount)
			break
		}

		logger.Logger.Info("recap backfill indexed", "count", result.IndexedCount)
	}

	// Phase 2: Incremental
	logger.Logger.Info("starting Recap Phase 2: Incremental", "since", lastSince)

	bo.Reset()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		result, err := indexRecapsUsecase.ExecuteIncremental(ctx, lastSince, config.RecapIndexBatchSize)
		if err != nil {
			recordError(ctx, "recap_incremental")
			delay := bo.NextBackOff()
			logger.Logger.Error("recap incremental error, retrying", "err", err, "retry_in", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			continue
		}
		bo.Reset()
		recordBatch(ctx, "recap_incremental", result.IndexedCount, 0, time.Since(start))

		if result.IndexedCount > 0 {
			logger.Logger.Info("recap incremental indexed", "count", result.IndexedCount)
			lastSince = result.LastSince
		} else {
			logger.Logger.Info("no new recap genres")
		}

		select {
		case <-time.After(config.RecapIndexInterval):
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
