package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"pre-processor/config"
	"pre-processor/consumer"
	"pre-processor/driver"
	backend_api "pre-processor/driver/backend_api"
	"pre-processor/handler"
	"pre-processor/repository"
	"pre-processor/service"
	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	batchSize = 40
)

// Dependencies holds all application dependencies.
type Dependencies struct {
	DBPool           *pgxpool.Pool
	JobHandler       handler.JobHandler
	HealthHandler    handler.HealthHandler
	SummarizeHandler *handler.SummarizeHandler
	RedisConsumer    *consumer.Consumer
	Logger           *slog.Logger

	// Repositories (exposed for Connect-RPC server)
	APIRepo     repository.ExternalAPIRepository
	SummaryRepo repository.SummaryRepository
	ArticleRepo repository.ArticleRepository
	JobRepo     repository.SummarizeJobRepository
}

// BuildDependencies constructs all application dependencies.
// Returns a cleanup function that should be deferred.
func BuildDependencies(ctx context.Context, log *slog.Logger, otelEnabled bool) (*Dependencies, func(), error) {
	// Initialize database (still required for job queue and quality checker)
	dbPool, err := driver.Init(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Load application config
	cfg, err := config.LoadConfig()
	if err != nil {
		dbPool.Close()
		return nil, nil, err
	}

	// Initialize repositories — API mode or legacy DB mode
	var articleRepo repository.ArticleRepository
	var summaryRepo repository.SummaryRepository

	if backendAPIURL := os.Getenv("BACKEND_API_URL"); backendAPIURL != "" {
		// API mode: use Connect-RPC client for article/feed/summary repos.
		// DB is still required for job queue (Category C) and quality checker.
		serviceToken := readSecret("SERVICE_TOKEN")
		log.Info("Using backend API driver for article/feed/summary repos",
			"url", backendAPIURL,
		)
		client := backend_api.NewClient(backendAPIURL, serviceToken)
		articleRepo = backend_api.NewArticleRepository(client, dbPool)
		summaryRepo = backend_api.NewSummaryRepository(client)
	} else {
		// Legacy DB mode
		log.Info("Using legacy database driver for article/feed/summary repos")
		articleRepo = repository.NewArticleRepository(dbPool, log)
		summaryRepo = repository.NewSummaryRepository(dbPool, log)
	}

	apiRepo := repository.NewExternalAPIRepository(cfg, log)
	jobRepo := repository.NewSummarizeJobRepository(dbPool, log)

	// Initialize services
	articleSummarizerService := service.NewArticleSummarizerService(articleRepo, summaryRepo, apiRepo, log)
	qualityCheckerService := service.NewQualityCheckerService(summaryRepo, apiRepo, dbPool, log)
	healthCheckerService := service.NewHealthCheckerServiceWithFactory(cfg, cfg.NewsCreator.Host, log)
	articleSyncService := service.NewArticleSyncService(articleRepo, apiRepo, log)
	summarizeQueueWorker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log, batchSize)

	// Initialize health metrics collector
	contextLogger := logger.NewContextLoggerWithOTel(logger.LoadLoggerConfigFromEnv(), otelEnabled)
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	// Initialize handlers
	jobHandler := handler.NewJobHandler(
		ctx,
		articleSummarizerService,
		qualityCheckerService,
		articleSyncService,
		healthCheckerService,
		summarizeQueueWorker,
		batchSize,
		log,
	)

	healthHandler := handler.NewHealthHandler(healthCheckerService, metricsCollector, log)
	summarizeHandler := handler.NewSummarizeHandler(apiRepo, summaryRepo, articleRepo, jobRepo, log)

	// Initialize Redis Streams consumer
	redisConsumer := buildRedisConsumer(ctx, jobRepo, articleRepo, log)

	cleanup := func() {
		dbPool.Close()
	}

	return &Dependencies{
		DBPool:           dbPool,
		JobHandler:       jobHandler,
		HealthHandler:    healthHandler,
		SummarizeHandler: summarizeHandler,
		RedisConsumer:    redisConsumer,
		Logger:           log,
		APIRepo:          apiRepo,
		SummaryRepo:      summaryRepo,
		ArticleRepo:      articleRepo,
		JobRepo:          jobRepo,
	}, cleanup, nil
}

// readSecret reads a secret value, supporting both direct env var and _FILE suffix
// for Docker Secrets compatibility.
func readSecret(key string) string {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		content, err := os.ReadFile(filePath)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}
	return os.Getenv(key)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func buildRedisConsumer(ctx context.Context, jobRepo repository.SummarizeJobRepository, articleRepo repository.ArticleRepository, log *slog.Logger) *consumer.Consumer {
	consumerCfg := consumer.Config{
		RedisURL:      getEnvOrDefault("REDIS_STREAMS_URL", "redis://redis-streams:6379"),
		GroupName:     getEnvOrDefault("CONSUMER_GROUP", "pre-processor-group"),
		ConsumerName:  getEnvOrDefault("CONSUMER_NAME", "pre-processor-1"),
		StreamKey:     "alt:events:articles",
		BatchSize:     10,
		BlockTimeout:  5 * time.Second,
		ClaimIdleTime: 30 * time.Second,
		Enabled:       getEnvOrDefault("CONSUMER_ENABLED", "false") == "true",
	}

	summarizeServiceAdapter := consumer.NewSummarizeServiceAdapter(jobRepo, articleRepo, log)
	eventHandler := consumer.NewPreProcessorEventHandler(summarizeServiceAdapter, log)
	redisConsumer, err := consumer.NewConsumer(consumerCfg, eventHandler, log)
	if err != nil {
		log.Error("Failed to create Redis Streams consumer", "error", err)
		return nil
	}

	if err := redisConsumer.Start(ctx); err != nil {
		log.Error("Failed to start Redis Streams consumer", "error", err)
	} else {
		log.Info("Redis Streams consumer started",
			"stream", consumerCfg.StreamKey,
			"group", consumerCfg.GroupName,
			"enabled", consumerCfg.Enabled)
	}

	return redisConsumer
}
