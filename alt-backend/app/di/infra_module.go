package di

import (
	"alt/config"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/orchestrator/driver/search_indexer_connect"
	"alt/orchestrator/gateway/config_gateway"
	"alt/orchestrator/gateway/robots_txt_gateway"
	"alt/orchestrator/port/search_indexer_port"
	"alt/shared/driver/alt_db"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/gateway/datahub_gateway"
	"alt/utils"
	"alt/utils/rate_limiter"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InfraModule holds the infrastructure cmd/backend's domain modules share.
//
// It is deliberately not the union of everything the three binaries need.
// cmd/harvester and cmd/datahub build their own narrow set of drivers in
// container_harvester.go / container_datahub.go, because a shared "build
// everything" infra module would hand each process credentials and clients for
// surfaces it does not serve — and would reintroduce the dead wiring this
// split removes (plan R11).
type InfraModule struct {
	Config      *config.Config
	RateLimiter *rate_limiter.HostRateLimiter
	HTTPClient  *http.Client
	MQHubClient *mqhub_connect.Client

	// Shared drivers. AltDBRepository is still here: the article reads and
	// writes, the feed tables and the knowledge backfill are later ADR-000954
	// Wave 3 batches, so cmd/backend keeps a database pool until they land.
	AltDBRepository     *alt_db.AltDBRepository
	SearchIndexerDriver search_indexer_port.SearchIndexerPort
	RobotsTxtGateway    *robots_txt_gateway.RobotsTxtGateway

	// alt-data-hub client and the capability gateways built on it
	// (ADR-000954 Wave 3 batch 1, catalog §2.D / §2.E / §2.L).
	DataHubClient          datahubv1connect.DataHubServiceClient
	OgImageGateway         *datahub_gateway.OgImageGateway
	ImageProxyCacheGateway *datahub_gateway.ImageProxyCacheGateway
	ScrapingDomainGateway  *datahub_gateway.ScrapingDomainGateway
	DeclinedDomainGateway  *datahub_gateway.DeclinedDomainGateway

	Pool *pgxpool.Pool
}

// newInfraModule wires infrastructure components from the single Config
// instance the composition root (cmd/backend) already loaded and validated.
// It must not call config.NewConfig() itself — doing so created a second,
// independently-loaded Config that could silently diverge from the one main
// already validated and logged.
//
// KratosClient and EventPublisher used to be built here. They are not any
// more: their only consumers (DataHubService.GetSystemUser and the
// DataHubService article mutations) live in cmd/datahub now, so building them for the
// backend would hand that process an auth-hub token and an event-publishing
// client it has no surface to use.
func newInfraModule(pool *pgxpool.Pool, cfg *config.Config) *InfraModule {
	altDBRepository := alt_db.NewAltDBRepository(pool)

	// Rate limiter configuration is read through the config gateway; the port
	// itself has no consumer beyond this function, so it stays a local.
	configPort := config_gateway.NewConfigGateway(cfg)
	rateLimitConfig := configPort.GetRateLimitConfig()
	hostRateLimiter := rate_limiter.NewHostRateLimiter(rateLimitConfig.ExternalAPIInterval, rateLimitConfig.ExternalAPIBurst)

	// HTTP client
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()

	// Robots.txt gateway (used by multiple components)
	robotsTxtGw := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// MQ-Hub client. The backend uses it for on-the-fly tag generation only;
	// event publishing moved to cmd/datahub.
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	logMQHubWiringState("alt-backend", cfg.MQHub.Enabled, cfg.MQHub.ConnectURL)

	// Search indexer driver (shared between article search and feed search)
	searchIndexerDriver := search_indexer_connect.NewConnectSearchIndexerDriver(cfg.SearchIndexer.ConnectURL, "")

	// alt-data-hub client. Not optional and not lazily built: three of the
	// domain modules below take a gateway from it, and a failure to construct
	// one must stop the process rather than surface on a user request.
	dataHubClient := newDataHubClient("alt-backend")

	return &InfraModule{
		Config:              cfg,
		RateLimiter:         hostRateLimiter,
		HTTPClient:          httpClient,
		MQHubClient:         mqhubClient,
		AltDBRepository:     altDBRepository,
		SearchIndexerDriver: searchIndexerDriver,
		RobotsTxtGateway:    robotsTxtGw,

		DataHubClient:          dataHubClient,
		OgImageGateway:         datahub_gateway.NewOgImageGateway(dataHubClient),
		ImageProxyCacheGateway: datahub_gateway.NewImageProxyCacheGateway(dataHubClient),
		ScrapingDomainGateway:  datahub_gateway.NewScrapingDomainGateway(dataHubClient),
		DeclinedDomainGateway:  datahub_gateway.NewDeclinedDomainGateway(dataHubClient),

		Pool: pool,
	}
}

// logMQHubWiringState emits the loud enabled/disabled signal CLAUDE.md rule 8
// / .claude/rules/di-wiring.md require: mqhub_connect.Client silently no-ops
// every event publish when disabled, and without this log there was no way
// to tell "MQHUB_ENABLED=false" (intentional) apart from a forgotten config
// flag at startup.
//
// The binary name is part of the record because two of the three processes
// build an mq-hub client for different reasons (backend: tag generation,
// data-hub: event publishing), and their logs otherwise read identically.
func logMQHubWiringState(binary string, enabled bool, connectURL string) {
	if enabled {
		slog.Info("mqhub_enabled", "binary", binary, "connect_url", connectURL)
	} else {
		slog.Warn("mqhub_disabled", "binary", binary, "reason", "MQHUB_ENABLED=false; event publishing will no-op")
	}
}
