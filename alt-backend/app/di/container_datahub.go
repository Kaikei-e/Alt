package di

import (
	"log/slog"

	"alt/config"
	"alt/dataplane/driver/kratos_client"
	"alt/dataplane/gateway/fetch_recent_articles_gateway"
	"alt/dataplane/gateway/recap_articles_gateway"
	"alt/dataplane/gateway/tag_set_version_gateway"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/shared/driver/alt_db"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/driver/sovereign_client"
	"alt/shared/gateway/event_publisher_gateway"
	"alt/shared/gateway/fetch_articles_by_tag_gateway"
	"alt/shared/gateway/fetch_tag_cloud_gateway"
	"alt/shared/gateway/internal_article_gateway"
	"alt/shared/gateway/summary_version_gateway"
	"alt/shared/port/event_publisher_port"
	"alt/shared/usecase/create_summary_version_usecase"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DataHubComponents is cmd/datahub's component set: what
// alt.datahub.v1.DataHubService needs to serve pre-processor, search-indexer,
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

	// Identity lookup for /v1/internal/system-user.
	kratosCli := kratos_client.NewKratosClient(cfg.AuthHub.URL, cfg.Auth.BackendTokenSecret)

	// Event publishing.
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	logMQHubWiringState("alt-data-hub", cfg.MQHub.Enabled, cfg.MQHub.ConnectURL)
	eventPublisher := event_publisher_gateway.NewEventPublisherGateway(mqhubClient, slog.Default())

	// Knowledge event sink.
	sovereignEnabled := logSovereignWiringState("alt-data-hub", cfg.Sovereign.URL, cfg.AppEnv)
	if !sovereignEnabled {
		panic("SOVEREIGN_URL is required for alt-data-hub in every environment — " +
			"versioned summary/tag-set artifacts would be written with no knowledge event appended")
	}
	sovereignCli := sovereign_client.NewClient(cfg.Sovereign.URL, sovereignEnabled)

	// One gateway instance satisfies every required and optional port of
	// datahubapi.NewHandler.
	internalArticleGw := internal_article_gateway.NewGateway(altDB)

	// Versioned artifacts.
	summaryVersionGw := summary_version_gateway.NewGateway(altDB)
	createSummaryVersionUC := create_summary_version_usecase.NewCreateSummaryVersionUsecase(
		summaryVersionGw, sovereignCli, summaryVersionGw,
	)
	tagSetVersionGw := tag_set_version_gateway.NewGateway(altDB)
	createTagSetVersionUC := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(
		tagSetVersionGw, sovereignCli, tagSetVersionGw,
	)

	// RAG tool reads.
	fetchTagCloudGw := fetch_tag_cloud_gateway.NewFetchTagCloudGateway(altDB)
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(fetchTagCloudGw, tagCloudCacheTTL)
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
	}
}
