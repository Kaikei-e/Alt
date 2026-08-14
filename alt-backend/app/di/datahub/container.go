// Package datahub is cmd/datahub's composition root.
//
// It is a package of its own, separate from alt/di, since ADR-000954 Wave 3
// batch 6 — and the reason is the same one that split the binaries. This is
// the only root that constructs an *alt_db.AltDBRepository, and Go's unit of
// linkage is the package: leaving it in alt/di would have put
// alt/shared/driver/alt_db in the dependency graph of cmd/backend and
// cmd/harvester too, since both import alt/di for their own roots. Wave 3's
// exit condition is that those two binaries *cannot* reach the database, and
// with one shared di package that could only ever have been a convention.
//
// It imports alt/di rather than duplicating it: the wiring-state loggers and
// the tag-cloud cache window are shared with the other two roots, and two
// copies of a "which binary emitted this" log helper is how the three
// processes' log lines start disagreeing.
package datahub

import (
	"log/slog"

	"alt/config"
	"alt/dataplane/driver/kratos_client"
	"alt/dataplane/gateway/datahub_capability_gateway"
	"alt/dataplane/gateway/fetch_articles_by_tag_gateway"
	"alt/dataplane/gateway/fetch_recent_articles_gateway"
	"alt/dataplane/gateway/fetch_tag_cloud_gateway"
	"alt/dataplane/gateway/internal_article_gateway"
	"alt/dataplane/gateway/recap_articles_gateway"
	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/dataplane/usecase/outbox_usecase"
	"alt/dataplane/usecase/push_delivery_usecase"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/di"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/shared/driver/alt_db"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/driver/sovereign_client"
	"alt/shared/gateway/event_publisher_gateway"
	"alt/shared/port/event_publisher_port"
	"alt/shared/usecase/create_summary_version_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DataHubComponents is cmd/datahub's component set: what
// services.datahub.v1.DataHubService needs to serve pre-processor, search-indexer,
// tag-generator, rag-orchestrator and recap-worker over mTLS.
//
// It builds no crawler, no search indexer, no image pipeline and no admin
// surface. KratosClient and EventPublisher live here and nowhere else — they
// had exactly one consumer each, both of which moved to this binary.
type DataHubComponents struct {
	Config *config.Config

	// Shared driver
	AltDBRepository *alt_db.AltDBRepository

	// DataHubService.GetSystemUser — the former GET /v1/internal/system-user
	// (ADR-000954 D6).
	KratosClient kratos_client.KratosClient

	// DataHubService article mutations publish through mq-hub.
	MQHubClient    *mqhub_connect.Client
	EventPublisher event_publisher_port.EventPublisherPort

	// Knowledge event sink for versioned artifacts.
	SovereignClient *sovereign_client.Client

	// The single gateway instance behind every DataHubService port.
	InternalArticleGateway *internal_article_gateway.Gateway

	// Versioned artifacts (append-first: summaries and tag sets are versioned,
	// never overwritten).
	CreateSummaryVersionUsecase *create_summary_version_usecase.CreateSummaryVersionUsecase
	CreateTagSetVersionUsecase  *create_tag_set_version_usecase.CreateTagSetVersionUsecase

	// RAG tool reads
	FetchTagCloudUsecase      *fetch_tag_cloud_usecase.FetchTagCloudUsecase
	FetchArticlesByTagUsecase *fetch_articles_by_tag_usecase.FetchArticlesByTagUsecase

	// Recap / recent article reads
	RecapArticlesUsecase       *recap_articles_usecase.RecapArticlesUsecase
	FetchRecentArticlesUsecase *fetch_recent_articles_usecase.FetchRecentArticlesUsecase

	// ADR-000954 Wave 3 batch 1 (catalog §2.A / §2.D / §2.E / §2.L / §2.O).
	//
	// The outbox gets a usecase because it has a state machine to enforce;
	// the other four are reads and single-statement writes whose invariants
	// are already in the SQL, so they go straight from handler to gateway the
	// way the phase 1-4 ports do.
	OutboxUsecase          *outbox_usecase.OutboxUsecase
	OgImageGateway         datahub_capability_port.OgImagePort
	ImageProxyCacheGateway datahub_capability_port.ImageProxyCachePort
	ScrapingPolicyGateway  datahub_capability_port.ScrapingPolicyPort
	AutoFulltextGateway    datahub_capability_port.AutoFulltextPort

	// ADR-000954 Wave 3 batch 2 (catalog §2.B / §2.C / §2.N).
	//
	// The article write gets no usecase even though it is the heaviest
	// transaction in the batch: the articles upsert and the outbox insert are
	// already one statement pair inside one driver method, so a usecase would
	// only forward. What made the outbox need one was a state machine spread
	// across several driver calls; this has none.
	ArticleWriteGateway      datahub_capability_port.ArticleWritePort
	ArticleReadGateway       datahub_capability_port.ArticleReadPort
	KnowledgeBackfillGateway datahub_capability_port.KnowledgeBackfillPort

	// ADR-000954 Wave 3 batch 3 (catalog §2.F / §2.G / §2.H).
	//
	// The availability gateway is where the batch's one merged capability
	// lands: RecordFeedLinkFailure holds the increment and the auto-disable in
	// one transaction, which is exactly the read-modify-write the collector
	// used to run across a process boundary (catalog §4-4).
	FeedLinkGateway             datahub_capability_port.FeedLinkPort
	FeedLinkAvailabilityGateway datahub_capability_port.FeedLinkAvailabilityPort
	FeedGateway                 datahub_capability_port.FeedPort

	// ADR-000954 Wave 3 batch 4 (catalog §2.I / §2.J).
	//
	// Neither gets a usecase. The read-state writes each hold their invariant
	// in one statement or one driver transaction, and the tag reads have none
	// at all — what made the outbox need a usecase was a state machine spread
	// across several driver calls, and there is nothing like that here.
	ReadStateGateway datahub_capability_port.ReadStatePort
	TagReadGateway   datahub_capability_port.TagReadPort

	// ADR-000954 Wave 3 batch 5 (catalog §2.K / §2.M).
	//
	// The version gateways are where this binary's reason for existing is most
	// visible: MarkSuperseded holds a per-article pg_advisory_xact_lock across
	// a read and an update, and an advisory xact lock cannot span a round trip.
	// Neither gets a usecase — the caller still orders the DB write against the
	// sovereign append, because talking to sovereign is its business (D4).
	SummaryVersionCapabilityGateway datahub_capability_port.SummaryVersionPort
	TagSetVersionCapabilityGateway  datahub_capability_port.TagSetVersionPort
	StatsGateway                    datahub_capability_port.StatsPort

	// ADR-000954 Wave 3 batch 6 (catalog §2.J / §2.C) — the last two.
	//
	// TagTrailGateway is the paged Tag Trail read whose Wave 2 wire shape
	// could not express the caller's cursor, and ArticleRefGateway is the
	// recall rail's projection fallback. Neither gets a usecase: one is a
	// query and the other is a single row, and there is no state machine
	// between them.
	TagTrailGateway   datahub_capability_port.TagTrailPort
	ArticleRefGateway datahub_capability_port.ArticleRefPort

	// Web Push storage: push_subscriptions (one row per device) and
	// push_deliveries (the dispatcher's queue, one row per notification per
	// device).
	//
	// The delivery queue gets a usecase for the same reason the outbox did —
	// a state machine spread across several driver calls — and the
	// subscription table does not, because each of its operations is a single
	// statement whose invariants are already in the SQL.
	PushSubscriptionGateway datahub_capability_port.PushSubscriptionPort
	PushDeliveryUsecase     *push_delivery_usecase.PushDeliveryUsecase
}

// NewDataHubComponents is cmd/datahub's composition root.
//
// Both optional-looking clients here are required, and both fail loudly rather
// than degrading:
//
//   - The mq-hub client no-ops every publish when disabled, which would make
//     the article RPCs answer 200 while emitting nothing. Outside development,
//     config.ValidateDataHubConfig has already rejected MQHUB_ENABLED=false;
//     this root still logs the resolved state at startup (CLAUDE.md rule 8).
//   - The sovereign client no-ops every knowledge-event append when
//     SOVEREIGN_URL is unset, which silently breaks the append-first
//     invariant. Required in every environment for this binary.
func NewDataHubComponents(pool *pgxpool.Pool, cfg *config.Config) *DataHubComponents {
	altDB := alt_db.NewAltDBRepository(pool)

	// Identity lookup for /v1/internal/system-user. The bearer is
	// INTERNAL_AUTH_SECRET, which is what auth-hub's /internal group is keyed
	// on — handing it BackendTokenSecret instead puts the HS256 signing key in
	// a plaintext header and gets 403 on every call, since auth-hub refuses to
	// start with the two secrets equal.
	kratosCli := kratos_client.NewKratosClient(cfg.AuthHub.URL, cfg.Auth.InternalAuthSecret)

	// Event publishing.
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	di.LogMQHubWiringState("alt-data-hub", cfg.MQHub.Enabled, cfg.MQHub.ConnectURL)
	eventPublisher := event_publisher_gateway.NewEventPublisherGateway(mqhubClient, slog.Default())

	// Knowledge event sink.
	sovereignEnabled := di.LogSovereignWiringState("alt-data-hub", cfg.Sovereign.URL, cfg.AppEnv)
	if !sovereignEnabled {
		panic("SOVEREIGN_URL is required for alt-data-hub in every environment — " +
			"versioned summary/tag-set artifacts would be written with no knowledge event appended")
	}
	sovereignCli := sovereign_client.NewClient(cfg.Sovereign.URL, sovereignEnabled)

	// One gateway instance satisfies every required and optional port of
	// datahubapi.NewHandler.
	internalArticleGw := internal_article_gateway.NewGateway(altDB)

	// Versioned artifacts.
	//
	// The same two gateways serve the DataHubService procedures and the
	// in-process usecases behind SaveArticleSummary / UpsertArticleTags. One
	// object per table, so a version written through the RPC and one written
	// through the chained usecase take the identical path, advisory lock
	// included — and the capability port names its methods after the artifact
	// precisely so that summary_version_port and tag_set_version_port are
	// satisfied by the same type, with no adapter in between.
	summaryVersionGw := datahub_capability_gateway.NewSummaryVersionGateway(altDB)
	tagSetVersionGw := datahub_capability_gateway.NewTagSetVersionGateway(altDB)
	createSummaryVersionUC := create_summary_version_usecase.NewCreateSummaryVersionUsecase(
		summaryVersionGw, sovereignCli, summaryVersionGw,
	)
	createTagSetVersionUC := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(
		tagSetVersionGw, sovereignCli, tagSetVersionGw,
	)

	// RAG tool reads.
	fetchTagCloudGw := fetch_tag_cloud_gateway.NewFetchTagCloudGateway(altDB)
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(fetchTagCloudGw, di.TagCloudCacheTTL)
	fetchArticlesByTagGw := fetch_articles_by_tag_gateway.NewFetchArticlesByTagGateway(altDB)
	fetchArticlesByTagUC := fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(fetchArticlesByTagGw)

	// Recap reads.
	recapArticlesGw := recap_articles_gateway.NewGateway(altDB)
	recapArticlesUC := recap_articles_usecase.NewRecapArticlesUsecase(recapArticlesGw, recap_articles_usecase.Config{
		DefaultPageSize: cfg.Recap.DefaultPageSize,
		MaxPageSize:     cfg.Recap.MaxPageSize,
		MaxRangeDays:    cfg.Recap.MaxRangeDays,
	})

	// Recent articles for rag-orchestrator's temporal topics.
	fetchRecentArticlesGw := fetch_recent_articles_gateway.NewFetchRecentArticlesGateway(pool)
	fetchRecentArticlesUC := fetch_recent_articles_usecase.NewFetchRecentArticlesUsecase(fetchRecentArticlesGw)

	// ADR-000954 Wave 3 batch 1 capabilities. Built unconditionally: with the
	// callers' own database pools gone, these are the only route alt-backend
	// and alt-harvester have to these tables, so there is no configuration
	// under which leaving one unwired is a valid deployment.
	outboxUC := outbox_usecase.NewOutboxUsecase(datahub_capability_gateway.NewOutboxGateway(altDB))
	ogImageGw := datahub_capability_gateway.NewOgImageGateway(altDB)
	imageProxyCacheGw := datahub_capability_gateway.NewImageProxyCacheGateway(altDB)
	scrapingPolicyGw := datahub_capability_gateway.NewScrapingPolicyGateway(altDB)
	autoFulltextGw := datahub_capability_gateway.NewAutoFulltextGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "outbox,og_image,image_proxy_cache,scraping_policy,auto_fulltext",
		"procedures", 23,
		"adr", "ADR-000954 Wave 3 batch 1")

	// ADR-000954 Wave 3 batch 2 capabilities. Same reasoning: after this batch
	// alt-backend has no database pool for articles, so there is no deployment
	// in which leaving one of these unwired is valid.
	articleWriteGw := datahub_capability_gateway.NewArticleWriteGateway(altDB)
	articleReadGw := datahub_capability_gateway.NewArticleReadGateway(altDB)
	knowledgeBackfillGw := datahub_capability_gateway.NewKnowledgeBackfillGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "article_write,article_read,knowledge_backfill",
		"procedures", 13,
		"adr", "ADR-000954 Wave 3 batch 2")

	// ADR-000954 Wave 3 batch 3 capabilities. Same reasoning again: after this
	// batch neither alt-backend nor alt-harvester has a pool for feed_links,
	// feed_link_availability or feeds.
	feedLinkGw := datahub_capability_gateway.NewFeedLinkGateway(altDB)
	feedLinkAvailabilityGw := datahub_capability_gateway.NewFeedLinkAvailabilityGateway(altDB)
	feedGw := datahub_capability_gateway.NewFeedGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "feed_link,feed_link_availability,feed",
		"procedures", 24,
		"adr", "ADR-000954 Wave 3 batch 3")

	// ADR-000954 Wave 3 batch 4 capabilities. Same reasoning once more: after
	// this batch alt-backend has no pool for read_status,
	// user_feed_subscriptions, favorite_feeds or the tag tables.
	readStateGw := datahub_capability_gateway.NewReadStateGateway(altDB)
	tagReadGw := datahub_capability_gateway.NewTagReadGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "read_state,tag_read",
		"procedures", 15,
		"adr", "ADR-000954 Wave 3 batch 4")

	// ADR-000954 Wave 3 batch 5 capabilities. Same reasoning to the end: after
	// this batch alt-backend has no pool for summary_versions,
	// tag_set_versions or any of the dashboard counts.
	statsGw := datahub_capability_gateway.NewStatsGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "summary_version,tag_set_version,stats",
		"procedures", 14,
		"adr", "ADR-000954 Wave 3 batch 5")

	// ADR-000954 Wave 3 batch 6 capabilities — the two that close the wave.
	// After these, alt-backend opens no database pool at all, so "leaving one
	// unwired" is not a degraded deployment but a missing feature that still
	// answers 200.
	tagTrailGw := datahub_capability_gateway.NewTagTrailGateway(altDB)
	articleRefGw := datahub_capability_gateway.NewArticleRefGateway(altDB)
	slog.Info("datahub.wave3_capabilities_enabled",
		"groups", "tag_trail,article_ref",
		"procedures", 3,
		"adr", "ADR-000954 Wave 3 batch 6")

	// Web Push storage. Built unconditionally for the same reason the Wave 3
	// batches are: alt-backend has no database pool, so this is the only route
	// alt.push.v1.PushService has to a subscription, and there is no
	// configuration under which leaving it unwired is a valid deployment. The
	// feature flag for Web Push lives in front of the browser-facing service,
	// not here — a data hub that stored no subscription while reporting
	// healthy is the ADR-000928 shape.
	pushSubscriptionGw := datahub_capability_gateway.NewPushSubscriptionGateway(altDB)
	pushDeliveryUC := push_delivery_usecase.NewPushDeliveryUsecase(
		datahub_capability_gateway.NewPushDeliveryGateway(altDB))
	slog.Info("datahub.push_storage_enabled",
		"groups", "push_subscriptions,push_deliveries",
		"procedures", 10)

	return &DataHubComponents{
		Config:                      cfg,
		AltDBRepository:             altDB,
		KratosClient:                kratosCli,
		MQHubClient:                 mqhubClient,
		EventPublisher:              eventPublisher,
		SovereignClient:             sovereignCli,
		InternalArticleGateway:      internalArticleGw,
		CreateSummaryVersionUsecase: createSummaryVersionUC,
		CreateTagSetVersionUsecase:  createTagSetVersionUC,
		FetchTagCloudUsecase:        fetchTagCloudUC,
		FetchArticlesByTagUsecase:   fetchArticlesByTagUC,
		RecapArticlesUsecase:        recapArticlesUC,
		FetchRecentArticlesUsecase:  fetchRecentArticlesUC,

		OutboxUsecase:          outboxUC,
		OgImageGateway:         ogImageGw,
		ImageProxyCacheGateway: imageProxyCacheGw,
		ScrapingPolicyGateway:  scrapingPolicyGw,
		AutoFulltextGateway:    autoFulltextGw,

		ArticleWriteGateway:      articleWriteGw,
		ArticleReadGateway:       articleReadGw,
		KnowledgeBackfillGateway: knowledgeBackfillGw,

		FeedLinkGateway:             feedLinkGw,
		FeedLinkAvailabilityGateway: feedLinkAvailabilityGw,
		FeedGateway:                 feedGw,

		ReadStateGateway: readStateGw,
		TagReadGateway:   tagReadGw,

		SummaryVersionCapabilityGateway: summaryVersionGw,
		TagSetVersionCapabilityGateway:  tagSetVersionGw,
		StatsGateway:                    statsGw,

		TagTrailGateway:   tagTrailGw,
		ArticleRefGateway: articleRefGw,

		PushSubscriptionGateway: pushSubscriptionGw,
		PushDeliveryUsecase:     pushDeliveryUC,
	}
}
