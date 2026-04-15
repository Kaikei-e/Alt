package di

import (
	"alt/config"
	"alt/driver/alt_db"
	"alt/driver/kratos_client"
	"alt/driver/mqhub_connect"
	"alt/driver/search_indexer_connect"
	"alt/gateway/config_gateway"
	"alt/gateway/error_handler_gateway"
	"alt/gateway/event_publisher_gateway"
	"alt/gateway/rate_limiter_gateway"
	"alt/gateway/robots_txt_gateway"
	"alt/port/config_port"
	"alt/port/error_handler_port"
	"alt/port/event_publisher_port"
	"alt/port/rate_limiter_port"
	"alt/port/search_indexer_port"
	"alt/utils"
	"alt/utils/rate_limiter"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InfraModule holds all infrastructure-level components shared across domain modules.
type InfraModule struct {
	Config          *config.Config
	ConfigPort      config_port.ConfigPort
	ErrorHandler    error_handler_port.ErrorHandlerPort
	RateLimiter     *rate_limiter.HostRateLimiter
	RateLimiterPort rate_limiter_port.RateLimiterPort
	HTTPClient      *http.Client
	KratosClient    kratos_client.KratosClient
	MQHubClient     *mqhub_connect.Client
	EventPublisher  event_publisher_port.EventPublisherPort

	// Shared drivers
	AltDBRepository     *alt_db.AltDBRepository
	SearchIndexerDriver search_indexer_port.SearchIndexerPort
	RobotsTxtGateway    *robots_txt_gateway.RobotsTxtGateway

	Pool *pgxpool.Pool
}

func newInfraModule(pool *pgxpool.Pool) *InfraModule {
	altDBRepository := alt_db.NewAltDBRepository(pool)

	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Create port implementations
	configPort := config_gateway.NewConfigGateway(cfg)
	errorHandlerPort := error_handler_gateway.NewErrorHandlerGateway()

	// Create rate limiter with configuration from port
	rateLimitConfig := configPort.GetRateLimitConfig()
	hostRateLimiter := rate_limiter.NewHostRateLimiter(rateLimitConfig.ExternalAPIInterval, rateLimitConfig.ExternalAPIBurst)
	rateLimiterPort := rate_limiter_gateway.NewRateLimiterGateway(hostRateLimiter)

	// HTTP client
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()

	// Robots.txt gateway (used by multiple components)
	robotsTxtGw := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// MQ-Hub event publisher (optional, fail-open if disabled)
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	eventPublisherGw := event_publisher_gateway.NewEventPublisherGateway(mqhubClient, slog.Default())

	// Auth-hub client for identity management (abstracts Kratos)
	kratosClientImpl := kratos_client.NewKratosClient(cfg.AuthHub.URL, cfg.Auth.BackendTokenSecret)

	// Search indexer driver (shared between article search and feed search)
	searchIndexerDriver := search_indexer_connect.NewConnectSearchIndexerDriver(cfg.SearchIndexer.ConnectURL, "")

	return &InfraModule{
		Config:              cfg,
		ConfigPort:          configPort,
		ErrorHandler:        errorHandlerPort,
		RateLimiter:         hostRateLimiter,
		RateLimiterPort:     rateLimiterPort,
		HTTPClient:          httpClient,
		KratosClient:        kratosClientImpl,
		MQHubClient:         mqhubClient,
		EventPublisher:      eventPublisherGw,
		AltDBRepository:     altDBRepository,
		SearchIndexerDriver: searchIndexerDriver,
		RobotsTxtGateway:    robotsTxtGw,
		Pool:                pool,
	}
}
