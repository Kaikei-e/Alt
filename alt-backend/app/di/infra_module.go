package di

import (
	"alt/config"
	"alt/domain"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/orchestrator/driver/search_indexer_connect"
	"alt/orchestrator/gateway/config_gateway"
	"alt/orchestrator/gateway/robots_txt_gateway"
	"alt/orchestrator/port/push_port"
	"alt/orchestrator/port/search_indexer_port"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/gateway/datahub_gateway"
	"alt/utils"
	"alt/utils/rate_limiter"
	"log/slog"
	"net/http"
)

// InfraModule holds the infrastructure cmd/backend's domain modules share.
//
// It is deliberately not the union of everything the three binaries need.
// cmd/harvester and cmd/datahub build their own narrow set of drivers in
// container_harvester.go / di/datahub, because a shared "build everything"
// infra module would hand each process credentials and clients for
// surfaces it does not serve — and would reintroduce the dead wiring this
// split removes (plan R11).
type InfraModule struct {
	Config      *config.Config
	RateLimiter *rate_limiter.HostRateLimiter
	HTTPClient  *http.Client
	MQHubClient *mqhub_connect.Client

	// RateLimiterCoordinator is the process-wide arbiter handle. Held on the
	// module because the image proxy runs its own rate-limit class (1s CDN
	// reads) and must be built from the same coordinator — two coordinators
	// would mean two connection pools and, worse, two independent startup mode
	// logs for one process (ADR-000954 review, weakness 5).
	RateLimiterCoordinator *HostRateLimiterCoordinator

	// There is no AltDBRepository field and no pool, since ADR-000954 Wave 3
	// batch 6. The two reads that kept one here through batch 5 — the Tag
	// Trail's paged articles and the recall rail's article fallback — became
	// procedures of their own, so cmd/backend reaches alt_db only through
	// alt-data-hub. That is Wave 3's exit condition, and it is asserted rather
	// than merely achieved: cmd/backend's dependency graph contains neither
	// alt_db nor pgx (see import_boundary_test.go).
	SearchIndexerDriver search_indexer_port.SearchIndexerPort
	RobotsTxtGateway    *robots_txt_gateway.RobotsTxtGateway

	// alt-data-hub client and the capability gateways built on it
	// (ADR-000954 Wave 3 batch 1, catalog §2.D / §2.E / §2.L; batch 2,
	// catalog §2.B / §2.C / §2.N).
	DataHubClient          datahubv1connect.DataHubServiceClient
	OgImageGateway         *datahub_gateway.OgImageGateway
	ImageProxyCacheGateway *datahub_gateway.ImageProxyCacheGateway
	ScrapingDomainGateway  *datahub_gateway.ScrapingDomainGateway
	DeclinedDomainGateway  *datahub_gateway.DeclinedDomainGateway

	// Batch 2. Four gateways rather than one because they are grouped by the
	// question they answer — archive an article, page a timeline, hydrate ids,
	// resolve a URL — and a single "article gateway" would hand every consumer
	// the whole surface.
	ArticleStoreGateway      *datahub_gateway.ArticleStoreGateway
	ArticleCursorGateway     *datahub_gateway.ArticleCursorGateway
	ArticleBatchGateway      *datahub_gateway.ArticleBatchGateway
	LatestArticleGateway     *datahub_gateway.LatestArticleGateway
	ArticleURLLookupGateway  *datahub_gateway.ArticleURLLookupGateway
	KnowledgeBackfillGateway *datahub_gateway.KnowledgeBackfillGateway

	// Batch 3 (catalog §2.F / §2.G / §2.H). Three gateways for three tables:
	// the subscription list, its poll health, and what polling produced.
	// FeedLinkAvailabilityGateway carries the auto-disable threshold, which is
	// operational policy and therefore lives on this side of the boundary
	// (catalog §4-4).
	FeedLinkGateway             *datahub_gateway.FeedLinkGateway
	FeedLinkAvailabilityGateway *datahub_gateway.FeedLinkAvailabilityGateway
	FeedGateway                 *datahub_gateway.FeedGateway

	// ADR-000954 Wave 3 batch 4 (capability catalog §2.I / §2.J): the per-user
	// state beside the feed list, and every tag read.
	//
	// ReadStateGateway satisfies four ports at once — read marks,
	// subscriptions, favourites — because they are one question about one
	// tenant asked four ways. TagGateway carries the tag write-back as well as
	// the reads, which is what lets fetch_article_tags_gateway stop having two
	// sources for one story.
	ReadStateGateway *datahub_gateway.ReadStateGateway
	TagGateway       *datahub_gateway.TagGateway

	// ADR-000954 Wave 3 batch 5 (capability catalog §2.K / §2.M): the
	// versioned artifacts and the dashboard numbers — the last two groups
	// cmd/backend read from its own pool.
	//
	// VersionGateway satisfies all seven version ports at once, summary and
	// tag-set, because they are one question about two tables. StatsGateway is
	// held by the thin port adapters in orchestrator/gateway rather than by a
	// usecase, since every one of those ports declares a method called Execute
	// and no single type can satisfy them all.
	VersionGateway *datahub_gateway.VersionGateway
	StatsGateway   *datahub_gateway.StatsGateway

	// ADR-000954 Wave 3 batch 6 (capability catalog §2.J / §2.C). TagGateway
	// grew the Tag Trail's two paged reads in the same batch, so the only new
	// field here is the recall rail's fallback — the last read this binary
	// performed against a database it owned.
	ArticleRefGateway *datahub_gateway.ArticleRefGateway

	// PushSubscriptionGateway is alt.push.v1.PushService's only route to
	// push_subscriptions.
	//
	// Declared as the port rather than as *datahub_gateway.PushSubscriptionGateway
	// so that an unwired module fails the nil check in push.NewHandler. A
	// concrete pointer field would arrive there as a non-nil interface holding a
	// nil pointer, which is precisely the shape rule 8 exists to prevent: the
	// guard would pass and the first user to grant notification permission would
	// take the panic instead of the composition root.
	PushSubscriptionGateway push_port.PushSubscriptionPort
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
func newInfraModule(cfg *config.Config) *InfraModule {
	// Rate limiter configuration is read through the config gateway; the port
	// itself has no consumer beyond this function, so it stays a local.
	configPort := config_gateway.NewConfigGateway(cfg)
	rateLimitConfig := configPort.GetRateLimitConfig()
	// The interval is a promise to the publisher's server, not to this
	// process. cmd/backend fetches the same hosts cmd/harvester's hourly
	// collector and og-image backfill fetch, so the two coordinate through a
	// shared arbiter when one is configured, and say loudly at startup when
	// one is not (ADR-000954 review, weakness 5).
	rateLimiterCoordinator := NewHostRateLimiterCoordinator("alt-backend", cfg.RateLimit.CoordinationRedisURL)
	hostRateLimiter := rateLimiterCoordinator.Limiter(
		rate_limiter.NamespaceExternalAPI, rateLimitConfig.ExternalAPIInterval, rateLimitConfig.ExternalAPIBurst)

	// HTTP client
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()

	// Robots.txt gateway (used by multiple components)
	robotsTxtGw := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// MQ-Hub client. The backend uses it for on-the-fly tag generation only;
	// event publishing moved to cmd/datahub.
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	LogMQHubWiringState("alt-backend", cfg.MQHub.Enabled, cfg.MQHub.ConnectURL)

	// Search indexer driver (shared between article search and feed search)
	searchIndexerDriver := search_indexer_connect.NewConnectSearchIndexerDriver(cfg.SearchIndexer.ConnectURL, "")

	// alt-data-hub client. Not optional and not lazily built: three of the
	// domain modules below take a gateway from it, and a failure to construct
	// one must stop the process rather than surface on a user request.
	dataHubClient := newDataHubClient("alt-backend")

	return &InfraModule{
		Config:                 cfg,
		RateLimiter:            hostRateLimiter,
		RateLimiterCoordinator: rateLimiterCoordinator,
		HTTPClient:             httpClient,
		MQHubClient:            mqhubClient,
		SearchIndexerDriver:    searchIndexerDriver,
		RobotsTxtGateway:       robotsTxtGw,

		DataHubClient:          dataHubClient,
		OgImageGateway:         datahub_gateway.NewOgImageGateway(dataHubClient),
		ImageProxyCacheGateway: datahub_gateway.NewImageProxyCacheGateway(dataHubClient),
		ScrapingDomainGateway:  datahub_gateway.NewScrapingDomainGateway(dataHubClient),
		DeclinedDomainGateway:  datahub_gateway.NewDeclinedDomainGateway(dataHubClient),

		ArticleStoreGateway:      datahub_gateway.NewArticleStoreGateway(dataHubClient),
		ArticleCursorGateway:     datahub_gateway.NewArticleCursorGateway(dataHubClient),
		ArticleBatchGateway:      datahub_gateway.NewArticleBatchGateway(dataHubClient),
		LatestArticleGateway:     datahub_gateway.NewLatestArticleGateway(dataHubClient),
		ArticleURLLookupGateway:  datahub_gateway.NewArticleURLLookupGateway(dataHubClient),
		KnowledgeBackfillGateway: datahub_gateway.NewKnowledgeBackfillGateway(dataHubClient),

		FeedLinkGateway:             datahub_gateway.NewFeedLinkGateway(dataHubClient),
		FeedLinkAvailabilityGateway: datahub_gateway.NewFeedLinkAvailabilityGateway(dataHubClient, domain.DefaultMaxConsecutiveFailures),
		FeedGateway:                 datahub_gateway.NewFeedGateway(dataHubClient),

		ReadStateGateway: datahub_gateway.NewReadStateGateway(dataHubClient),
		TagGateway:       datahub_gateway.NewTagGateway(dataHubClient),

		VersionGateway: datahub_gateway.NewVersionGateway(dataHubClient),
		StatsGateway:   datahub_gateway.NewStatsGateway(dataHubClient),

		ArticleRefGateway: datahub_gateway.NewArticleRefGateway(dataHubClient),

		PushSubscriptionGateway: datahub_gateway.NewPushSubscriptionGateway(dataHubClient),
	}
}

// LogMQHubWiringState emits the loud enabled/disabled signal CLAUDE.md rule 8
// / .claude/rules/di-wiring.md require: mqhub_connect.Client silently no-ops
// every event publish when disabled, and without this log there was no way
// to tell "MQHUB_ENABLED=false" (intentional) apart from a forgotten config
// flag at startup.
//
// The binary name is part of the record because two of the three processes
// build an mq-hub client for different reasons (backend: tag generation,
// data-hub: event publishing), and their logs otherwise read identically.
func LogMQHubWiringState(binary string, enabled bool, connectURL string) {
	if enabled {
		slog.Info("mqhub_enabled", "binary", binary, "connect_url", connectURL)
	} else {
		slog.Warn("mqhub_disabled", "binary", binary, "reason", "MQHUB_ENABLED=false; event publishing will no-op")
	}
}
