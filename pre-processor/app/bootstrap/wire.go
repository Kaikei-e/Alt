package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"net/http"

	"pre-processor/config"
	"pre-processor/consumer"
	"pre-processor/driver"
	backend_api "pre-processor/driver/backend_api"
	"pre-processor/handler"
	"pre-processor/metrics"
	qualitychecker "pre-processor/quality-checker"
	"pre-processor/repository"
	"pre-processor/service"
	"pre-processor/tlsutil"
	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// buildBackendHTTPClient returns an *http.Client the Connect-RPC backend
// client uses to reach alt-backend. When MTLS_ENFORCE=true the client is
// built with tlsutil.LoadClientConfig so it presents the pre-processor leaf
// cert on every handshake; otherwise http.DefaultClient is returned.
func buildBackendHTTPClient(log *slog.Logger) (*http.Client, error) {
	if os.Getenv("MTLS_ENFORCE") != "true" {
		return &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 4,
			},
		}, nil
	}
	tlsCfg, err := tlsutil.LoadClientConfig(
		os.Getenv("MTLS_CERT_FILE"),
		os.Getenv("MTLS_KEY_FILE"),
		os.Getenv("MTLS_CA_FILE"),
	)
	if err != nil {
		return nil, fmt.Errorf("backend mTLS client (fail-closed): %w", err)
	}
	if sn := os.Getenv("BACKEND_MTLS_SERVER_NAME"); sn != "" {
		tlsCfg.ServerName = sn
	}
	log.Info("backend API client: mTLS enforce enabled",
		"server_name", tlsCfg.ServerName,
	)
	// This client is shared by the backend_api driver (article/feed/summary
	// repos) and, since ADR-000954's DataHub client wiring fix, by
	// repository.ExternalAPIRepository's GetSystemUserID too — which runs on
	// a JobRunner ticker loop that reuses the same context.Context for every
	// tick and so never carries a per-run deadline of its own (see
	// orchestrator.JobRunner.run). Without Client.Timeout and the Transport
	// phase timeouts below, a peer that accepts the TCP/TLS handshake but
	// then stalls would hang this client's calls indefinitely and — because
	// JobRunner invokes r.fn synchronously — permanently wedge that job's
	// ticker loop. Values mirror utils.HTTPClientManager's defaultClient.
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}, nil
}

const (
	batchSize = 10
)

// Dependencies holds all application dependencies.
type Dependencies struct {
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

	// MetricsCollector owns the dedicated metrics listener. The relay's gauges
	// are registered on it; without a scrape target the numbers exist but
	// nobody can see a wedged relay.
	MetricsCollector *metrics.Collector
}

// buildMetricsCollector wires the dedicated metrics listener and registers the
// notification-outbox relay's gauges on it.
//
// The gauges deliberately do not live on the Echo API listener: that listener
// authenticates nothing beyond "who can open a socket", so a route on it is a
// new unauthenticated surface on the service API.
func buildMetricsCollector(
	cfg config.MetricsConfig,
	relayMetrics *metrics.OutboxRelayMetrics,
	log *slog.Logger,
) (*metrics.Collector, error) {
	if relayMetrics == nil {
		return nil, fmt.Errorf("metrics collector: outbox relay metrics are required")
	}

	collector, err := metrics.NewCollector(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("metrics collector: %w", err)
	}

	if err := collector.RegisterExporter("notification_outbox_relay", relayMetrics); err != nil {
		return nil, fmt.Errorf("metrics collector: %w", err)
	}

	return collector, nil
}

// buildNotificationRelay wires the notification_outbox relay over the same
// pre-processor-db pool the producer writes to and the same mTLS alt-data-hub
// client the article/feed/summary repositories already use.
//
// Every collaborator is required. There is no "relay disabled" branch: the
// producer writes outbox rows unconditionally as part of completing a job, so
// a pre-processor running without a relay is not a degraded mode, it is a
// backlog nobody drains.
func buildNotificationRelay(
	ppDBPool *pgxpool.Pool,
	client *backend_api.Client,
	relayMetrics *metrics.OutboxRelayMetrics,
	log *slog.Logger,
) (*service.NotificationRelay, error) {
	if ppDBPool == nil {
		return nil, fmt.Errorf("notification relay: pre-processor-db pool is required")
	}
	if client == nil {
		return nil, fmt.Errorf("notification relay: alt-data-hub client is required")
	}

	// locked_by has to identify the process, so a stuck row points at
	// something an operator can go look at.
	name, err := os.Hostname()
	if err != nil || name == "" {
		return nil, fmt.Errorf("notification relay: cannot determine hostname for locked_by: %w", err)
	}

	return service.NewNotificationRelay(
		repository.NewNotificationOutboxRepository(ppDBPool, log),
		backend_api.NewNotificationForwarder(client),
		relayMetrics,
		name,
		log,
	)
}

// BuildDependencies constructs all application dependencies.
// Returns a cleanup function that should be deferred.
func BuildDependencies(ctx context.Context, log *slog.Logger, otelEnabled bool) (*Dependencies, func(), error) {
	// BACKEND_API_URL is required — legacy alt-db mode has been removed.
	backendAPIURL := os.Getenv("BACKEND_API_URL")
	if backendAPIURL == "" {
		return nil, nil, fmt.Errorf("BACKEND_API_URL is required; legacy direct-DB mode has been removed")
	}

	// Initialize pre-processor-db (ADR-000246) — required for job queue and inoreader tables
	ppDBPool, err := driver.InitPreProcessorDB(ctx)
	if err != nil {
		log.Error("Failed to connect to pre-processor-db", "error", err)
		return nil, nil, err
	}
	log.Info("Using dedicated pre-processor-db for job queue and inoreader tables")
	ppDBPoolCleanup := func() { ppDBPool.Close() }

	// Load application config
	cfg, err := config.LoadConfig()
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, err
	}
	qualitychecker.Configure(cfg)

	// Initialize repositories — API mode via Connect-RPC to alt-backend.
	// Authentication is established at the TLS transport layer (mTLS).
	backendHTTPClient, err := buildBackendHTTPClient(log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, err
	}
	if mtlsURL := os.Getenv("BACKEND_API_MTLS_URL"); mtlsURL != "" && os.Getenv("MTLS_ENFORCE") == "true" {
		backendAPIURL = mtlsURL
	}
	log.Info("Using backend API driver for article/feed/summary repos",
		"url", backendAPIURL,
		"mtls_enforce", os.Getenv("MTLS_ENFORCE") == "true",
	)
	client := backend_api.NewClient(backendAPIURL, "", backendHTTPClient)
	articleRepo := backend_api.NewArticleRepository(client, ppDBPool)
	summaryRepo := backend_api.NewSummaryRepository(client)

	// GetSystemUserID (repository.ExternalAPIRepository) must reach
	// alt-data-hub over the same mTLS-configured client and resolved URL as
	// the backend_api driver above — passing cfg here and letting the
	// constructor rebuild its own client/URL from cfg.AltService.Host is
	// exactly the wiring bug ADR-000954 left behind (that field targets
	// alt-backend's plaintext operator listener, which does not serve
	// DataHubService).
	apiRepo := repository.NewExternalAPIRepository(cfg, log, backendHTTPClient, backendAPIURL)
	jobRepo := repository.NewSummarizeJobRepository(ppDBPool, log)

	// Initialize services
	articleSummarizerService := service.NewArticleSummarizerService(articleRepo, summaryRepo, apiRepo, log)
	qualityCheckerService := service.NewQualityCheckerService(summaryRepo, articleRepo, apiRepo, jobRepo, log)
	healthCheckerService := service.NewHealthCheckerServiceWithFactory(cfg, cfg.NewsCreator.Host, log)
	articleSyncService := service.NewArticleSyncService(articleRepo, apiRepo, log)
	summarizeQueueWorker := service.NewSummarizeQueueWorker(jobRepo, articleRepo, apiRepo, summaryRepo, log, batchSize)
	summarizeQueueWorker.SetConcurrency(cfg.SummarizeQueue.Concurrency)

	// Initialize health metrics collector
	contextLogger := logger.NewContextLoggerWithOTel(logger.LoadLoggerConfigFromEnv(), otelEnabled)
	metricsCollector := service.NewHealthMetricsCollector(contextLogger)

	outboxMetrics := metrics.NewOutboxRelayMetrics()
	notificationRelay, err := buildNotificationRelay(ppDBPool, client, outboxMetrics, log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, fmt.Errorf("failed to build notification relay: %w", err)
	}

	promCollector, err := buildMetricsCollector(cfg.Metrics, outboxMetrics, log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, fmt.Errorf("failed to build metrics collector: %w", err)
	}

	// Initialize handlers
	jobHandler := handler.NewJobHandler(
		ctx,
		articleSummarizerService,
		qualityCheckerService,
		articleSyncService,
		healthCheckerService,
		summarizeQueueWorker,
		notificationRelay,
		batchSize,
		log,
	)

	healthHandler := handler.NewHealthHandler(healthCheckerService, metricsCollector, log)
	summarizeHandler := handler.NewSummarizeHandler(apiRepo, summaryRepo, articleRepo, jobRepo, log)

	// Initialize Redis Streams consumer
	redisConsumer, err := buildRedisConsumer(ctx, jobRepo, articleRepo, summaryRepo, log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, fmt.Errorf("failed to build redis streams consumer: %w", err)
	}

	cleanup := func() {
		ppDBPoolCleanup()
	}

	return &Dependencies{
		JobHandler:       jobHandler,
		HealthHandler:    healthHandler,
		SummarizeHandler: summarizeHandler,
		RedisConsumer:    redisConsumer,
		Logger:           log,
		APIRepo:          apiRepo,
		SummaryRepo:      summaryRepo,
		ArticleRepo:      articleRepo,
		JobRepo:          jobRepo,
		MetricsCollector: promCollector,
	}, cleanup, nil
}

// readSecret reads a secret value, supporting both direct env var and _FILE suffix
// for Docker Secrets compatibility.
func readSecret(key string) string {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		content, err := os.ReadFile(filePath) // #nosec G304 -- filePath comes from trusted env var for Docker Secrets
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

func getEnvIntOrDefault(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

// buildRedisConsumer constructs and starts the Redis Streams consumer that
// drives event-driven summarization. Construction or start failures are
// returned to the caller (rather than logged-and-swallowed) so a broken
// consumer fails pre-processor startup instead of leaving the event-driven
// summarization path silently dead (CLAUDE.md rule 8). When CONSUMER_ENABLED
// is unset/false, NewConsumer/Start succeed as a deliberate no-op and log
// "consumer disabled, not starting" — that is the explicit, loud opt-out path.
func buildRedisConsumer(ctx context.Context, jobRepo repository.SummarizeJobRepository, articleRepo repository.ArticleRepository, summaryRepo repository.SummaryRepository, log *slog.Logger) (*consumer.Consumer, error) {
	consumerCfg := consumer.Config{
		RedisURL:      getEnvOrDefault("REDIS_STREAMS_URL", "redis://redis-streams:6379"),
		GroupName:     getEnvOrDefault("CONSUMER_GROUP", "pre-processor-group"),
		ConsumerName:  getEnvOrDefault("CONSUMER_NAME", "pre-processor-1"),
		StreamKey:     "alt:events:articles",
		BatchSize:     10,
		BlockTimeout:  5 * time.Second,
		ClaimIdleTime: 30 * time.Second,
		Enabled:       getEnvOrDefault("CONSUMER_ENABLED", "false") == "true",
		DLQStreamKey:  getEnvOrDefault("CONSUMER_DLQ_STREAM", "alt:events:articles:dlq"),
		MaxDeliveries: getEnvIntOrDefault("CONSUMER_MAX_DELIVERIES", 5),
	}

	summarizeServiceAdapter := consumer.NewSummarizeServiceAdapter(jobRepo, articleRepo, summaryRepo, log)
	eventHandler := consumer.NewPreProcessorEventHandler(summarizeServiceAdapter, log)
	redisConsumer, err := consumer.NewConsumer(consumerCfg, eventHandler, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis streams consumer: %w", err)
	}

	if err := redisConsumer.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start redis streams consumer: %w", err)
	}

	log.Info("Redis Streams consumer started",
		"stream", consumerCfg.StreamKey,
		"group", consumerCfg.GroupName,
		"enabled", consumerCfg.Enabled,
		"dlq_stream", consumerCfg.DLQStreamKey,
		"max_deliveries", consumerCfg.MaxDeliveries)

	return redisConsumer, nil
}
