package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

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
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
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
	sn := os.Getenv("BACKEND_MTLS_SERVER_NAME")
	if sn == "" {
		sn = "alt-data-hub"
	}
	tlsCfg.ServerName = sn
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
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
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
		repository.NewNotificationForwarder(client),
		relayMetrics,
		name,
		log,
	)
}

// ValidateServiceURL checks that a URL string is valid, non-empty, has a host,
// has no embedded user credentials (userinfo), and satisfies the HTTPS requirement
// when mTLS is enforced.
func ValidateServiceURL(rawURL string, requireHTTPS bool, serviceName string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("%s URL is required and cannot be empty", serviceName)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s URL is malformed", serviceName)
	}

	if u.Host == "" {
		return nil, fmt.Errorf("%s URL is missing a host", serviceName)
	}

	if u.User != nil {
		return nil, fmt.Errorf("%s URL must not contain user credentials/userinfo", serviceName)
	}

	if requireHTTPS {
		if u.Scheme != "https" {
			return nil, fmt.Errorf("%s URL must use https when mTLS is enforced", serviceName)
		}
	} else {
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("%s URL must use http or https", serviceName)
		}
	}

	return u, nil
}

// ResolveServiceURLs determines and validates the DataHub backend URL and NewsCreator URL
// before any database connection or network calls are made.
//
// When mtlsEnforced is true:
//   - DataHub URL MUST be provided by backendAPIMtlsURL (BACKEND_API_MTLS_URL) and MUST be HTTPS.
//     Silent fallback to plaintext backendAPIURL (BACKEND_API_URL) is forbidden.
//   - NewsCreator URL (from newsCreatorHost) MUST be HTTPS.
//
// When mtlsEnforced is false (development mode):
//   - DataHub URL uses backendAPIMtlsURL if set, else falls back to backendAPIURL (HTTP or HTTPS allowed).
//   - NewsCreator URL allows HTTP or HTTPS.
//
// Both URLs reject userinfo, empty strings, and malformed inputs fail-closed.
func ResolveServiceURLs(mtlsEnforced bool, backendAPIURL, backendAPIMtlsURL, newsCreatorHost string) (resolvedBackendURL, resolvedNewsURL string, err error) {
	var targetBackend string
	if mtlsEnforced {
		if strings.TrimSpace(backendAPIMtlsURL) == "" {
			return "", "", fmt.Errorf("BACKEND_API_MTLS_URL is required and must be nonempty HTTPS when MTLS_ENFORCE=true; cannot fallback to BACKEND_API_URL")
		}
		targetBackend = backendAPIMtlsURL
	} else {
		if strings.TrimSpace(backendAPIMtlsURL) != "" {
			targetBackend = backendAPIMtlsURL
		} else if strings.TrimSpace(backendAPIURL) != "" {
			targetBackend = backendAPIURL
		} else {
			return "", "", fmt.Errorf("BACKEND_API_URL is required; legacy direct-DB mode has been removed")
		}
	}

	uBackend, err := ValidateServiceURL(targetBackend, mtlsEnforced, "DataHub backend")
	if err != nil {
		return "", "", err
	}

	uNews, err := ValidateServiceURL(newsCreatorHost, mtlsEnforced, "NewsCreator")
	if err != nil {
		return "", "", err
	}

	return uBackend.String(), uNews.String(), nil
}

// BuildDependencies constructs all application dependencies.
// Returns a cleanup function that should be deferred.
func BuildDependencies(ctx context.Context, log *slog.Logger, otelEnabled bool) (*Dependencies, func(), error) {
	// 1. Load application config and resolve/validate service URLs BEFORE any database initialization
	// or network side-effects. In production (MTLS_ENFORCE=true), DataHub and NewsCreator must both be
	// nonempty HTTPS (fail-closed, no silent fallback).
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	mtlsEnforced := os.Getenv("MTLS_ENFORCE") == "true"
	backendAPIURL, newsURL, err := ResolveServiceURLs(
		mtlsEnforced,
		os.Getenv("BACKEND_API_URL"),
		os.Getenv("BACKEND_API_MTLS_URL"),
		cfg.NewsCreator.Host,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("service URL validation failed (fail-closed): %w", err)
	}
	cfg.NewsCreator.Host = newsURL

	// 2. Initialize pre-processor-db (ADR-000246) — required for job queue and inoreader tables
	ppDBPool, err := driver.InitPreProcessorDB(ctx)
	if err != nil {
		log.Error("Failed to connect to pre-processor-db", "error", err)
		return nil, nil, err
	}
	log.Info("Using dedicated pre-processor-db for job queue and inoreader tables")
	ppDBPoolCleanup := func() { ppDBPool.Close() }

	// Initialize repositories — API mode via Connect-RPC to alt-backend.
	// Authentication is established at the TLS transport layer (mTLS).
	backendHTTPClient, err := buildBackendHTTPClient(log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, err
	}
	log.Info("Using backend API driver for article/feed/summary repos",
		"url", backendAPIURL,
		"mtls_enforce", mtlsEnforced,
	)

	// Reuse the already-configured mTLS backend HTTP client as the DataHub
	// Connect-RPC transport — same cert rotation, same dial parameters, no
	// second LoadX509KeyPair call, no ppDBPoolCleanup leak on cert errors.
	// SNI for alt-data-hub is set by buildBackendHTTPClient via BACKEND_MTLS_SERVER_NAME.
	client := backend_api.NewClient(backendAPIURL, "", backendHTTPClient)

	articleRepo := repository.NewArticleRepository(client, ppDBPool)
	summaryRepo := repository.NewSummaryRepository(client)

	// GetSystemUserID (repository.ExternalAPIRepository) must reach
	// alt-data-hub over the same mTLS-configured client and resolved URL as
	// the backend_api driver above — passing cfg here and letting the
	// constructor rebuild its own client/URL from cfg.AltService.Host is
	// exactly the wiring bug ADR-000954 left behind (that field targets
	// alt-backend's plaintext operator listener, which does not serve
	// DataHubService).
	newsHTTPClient, err := buildNewsHTTPClient(cfg, log)
	if err != nil {
		ppDBPoolCleanup()
		return nil, nil, err
	}
	qualitychecker.Configure(cfg, newsHTTPClient)
	apiRepo := repository.NewExternalAPIRepository(cfg, log, backendHTTPClient, backendAPIURL, newsHTTPClient)
	jobRepo := repository.NewSummarizeJobRepository(ppDBPool, log)

	// Initialize services
	articleSummarizerService := service.NewArticleSummarizerService(articleRepo, summaryRepo, apiRepo, log)
	qualityCheckerService := service.NewQualityCheckerService(summaryRepo, articleRepo, apiRepo, jobRepo, log)
	// Inject the actual configured news client so the health checker validates
	// the same TLS/CA that the summarizer uses; uncertified factory transport
	// would accept wrong-CA responses.
	healthCheckerService := service.NewHealthCheckerServiceWithClient(newsHTTPClient, cfg.NewsCreator.Host, log)
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
	redisPassword, err := config.ResolveRedisPassword(log)
	if err != nil {
		return nil, fmt.Errorf("resolve redis password: %w", err)
	}

	consumerCfg := consumer.Config{
		RedisURL:      getEnvOrDefault("REDIS_STREAMS_URL", "redis://redis-streams:6379"),
		RedisPassword: redisPassword,
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

// buildNewsHTTPClient constructs the *http.Client for the news-creator API.
// It uses a dedicated SNI ("news-creator") distinct from the DataHub client
// (whose SNI is "alt-data-hub" / BACKEND_MTLS_SERVER_NAME) and honours
// cfg.NewsCreator.Timeout (default 600s) so streaming LLM calls are not cut
// short by a hard 30s ceiling. CheckRedirect is disabled fail-closed: internal
// services must not silently forward to unexpected hosts.
func buildNewsHTTPClient(cfg *config.Config, log *slog.Logger) (*http.Client, error) {
	timeout := cfg.NewsCreator.Timeout
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	if os.Getenv("MTLS_ENFORCE") != "true" {
		return &http.Client{
			Timeout:       timeout,
			CheckRedirect: noRedirect,
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
		return nil, fmt.Errorf("news mTLS client (fail-closed): %w", err)
	}
	// SNI must be "news-creator" — separate from the DataHub client whose SNI
	// is "alt-data-hub". NEWS_CREATOR_MTLS_SERVER_NAME overrides for non-standard deployments.
	sn := os.Getenv("NEWS_CREATOR_MTLS_SERVER_NAME")
	if sn == "" {
		sn = "news-creator"
	}
	tlsCfg.ServerName = sn
	log.Info("news-creator client: mTLS enforce enabled",
		"server_name", tlsCfg.ServerName,
		"timeout", timeout,
	)

	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: noRedirect,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			IdleConnTimeout:     30 * time.Second,
			MaxIdleConnsPerHost: 4,
		},
	}, nil
}
