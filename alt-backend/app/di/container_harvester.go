package di

import (
	"log/slog"
	"net/http"
	"time"

	"alt/adapter/augur_adapter"
	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/orchestrator/gateway/feed_link_domain_gateway"
	"alt/orchestrator/gateway/fetch_article_gateway"
	"alt/orchestrator/gateway/image_fetch_gateway"
	"alt/orchestrator/gateway/image_proxy_gateway"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/gateway/robots_txt_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/shared/driver/sovereign_client"
	"alt/shared/gateway/datahub_gateway"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils"
	"alt/utils/image_proxy"
	"alt/utils/rate_limiter"
)

// HarvesterComponents is cmd/harvester's component set: exactly what the
// scheduled jobs in orchestrator/job need, and nothing else.
//
// The jobs are the only consumers, so this is a flat struct rather than the
// module tree the backend keeps — there are no handlers to hold to a
// backward-compatible shape. What it omits matters more than what it has:
// no search indexer, no mq-hub client, no Kratos client, no event publisher,
// no Connect-RPC clients, no CSRF store. A job that starts reaching for one of
// those fails to compile here rather than silently no-op'ing at runtime.
type HarvesterComponents struct {
	Config *config.Config

	// There is no AltDBRepository field. The tag cloud read was the last one
	// that needed it, and ADR-000954 Wave 3 batch 5 moved it onto the same
	// gateway the backend uses (catalog §2.J). alt-harvester now reaches alt_db
	// only through alt-data-hub, which is the state Wave 3's exit condition
	// asks for: a job that reaches for a database fails to compile.

	// alt-data-hub client and the capability gateways built on it
	// (ADR-000954 Wave 3 batch 1, catalog §2.A / §2.D / §2.E / §2.L).
	DataHubClient          datahubv1connect.DataHubServiceClient
	OutboxGateway          *datahub_gateway.OutboxGateway
	OgImageGateway         *datahub_gateway.OgImageGateway
	ImageProxyCacheGateway *datahub_gateway.ImageProxyCacheGateway

	// hourly-feed-collector (ADR-000954 Wave 3 batch 3, catalog §2.F / §2.G /
	// §2.H). FeedCollectorGateway bundles the three capabilities the job uses;
	// the auto-disable threshold is baked into the availability half at
	// construction because it is this process's operational policy
	// (catalog §4-4).
	FeedCollectorGateway *harvesterFeedCollectorGateway
	// daily-scraping-policy
	ScrapingDomainUsecase *scraping_domain_usecase.ScrapingDomainUsecase

	// og-image-backfill
	FetchArticleGateway *fetch_article_gateway.FetchArticleGateway

	// ogp-image-warmer / og-image-backfill. Non-nil whenever the harvester
	// starts: NewHarvesterComponents refuses to build otherwise, because two
	// of the eight jobs exist only to run the image pipeline.
	ImageProxyUsecase *image_proxy_usecase.ImageProxyUsecase

	// Read model shared with the RAG tool surface. The harvester holds it for
	// completeness of the article read path; it does not run the tag-cloud
	// warmer (see RegisterHarvesterJobs).
	FetchTagCloudUsecase *fetch_tag_cloud_usecase.FetchTagCloudUsecase

	// outbox-worker
	RagIntegration  rag_integration_port.RagIntegrationPort
	SovereignClient *sovereign_client.Client
}

// NewHarvesterComponents is cmd/harvester's composition root.
//
// Two Rule 8 / Rule 9 differences from the backend's root, both driven by what
// the jobs do rather than what the config says in general:
//
//   - SOVEREIGN_URL must be set in every environment, not only production. The
//     outbox worker marks rows PROCESSED after handing their knowledge events
//     to the sovereign client; a disabled client drops those events silently,
//     which breaks the append-first invariant while every log still reads
//     "success". config.ValidateHarvesterConfig enforces it before we get here;
//     this root panics if that check was bypassed.
//   - The image proxy must be wired unless IMAGE_PROXY_ENABLED=false is
//     explicit. logImageProxyWiringState already panics on
//     "enabled but no secret"; for the harvester a deliberate disable is also
//     load-bearing, because RegisterHarvesterJobs then refuses to register the
//     two image jobs rather than ticking them into a warning every hour.
//
// There is no pool parameter. ADR-000954 Wave 3 batch 5 removed the
// harvester's last direct query and batch 6 removed the plumbing that carried
// a pool to it anyway: internal/bootstrap no longer opens one, the DB_* block
// is gone from this container's compose environment, and cmd/harvester's
// dependency graph contains neither alt_db nor pgx (di/import_boundary_test.go).
func NewHarvesterComponents(cfg *config.Config) *HarvesterComponents {
	// alt-data-hub client. Built before anything else because five of the
	// eight jobs cannot run without it, and a failure here must stop the
	// process rather than surface later as a handshake error on a scheduler
	// tick (CLAUDE.md rule 9).
	dataHubClient := newDataHubClient("alt-harvester")
	outboxGw := datahub_gateway.NewOutboxGateway(dataHubClient)
	ogImageGw := datahub_gateway.NewOgImageGateway(dataHubClient)
	imageProxyCacheGw := datahub_gateway.NewImageProxyCacheGateway(dataHubClient)
	scrapingDomainGw := datahub_gateway.NewScrapingDomainGateway(dataHubClient)
	feedLinkGw := datahub_gateway.NewFeedLinkGateway(dataHubClient)
	feedCollectorGw := &harvesterFeedCollectorGateway{
		FeedLinkGateway:             feedLinkGw,
		FeedGateway:                 datahub_gateway.NewFeedGateway(dataHubClient),
		FeedLinkAvailabilityGateway: datahub_gateway.NewFeedLinkAvailabilityGateway(dataHubClient, domain.DefaultMaxConsecutiveFailures),
	}

	// Shared external-fetch primitives. CLAUDE.md rule 2: this rate limiter is
	// process-local, so the harvester's effective rate against a publisher is
	// independent of the backend's — see the split ADR's R1 note.
	rateLimitInterval := cfg.RateLimit.ExternalAPIInterval
	hostRateLimiter := rate_limiter.NewHostRateLimiter(rateLimitInterval, cfg.RateLimit.ExternalAPIBurst)
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()
	robotsTxtGw := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// daily-scraping-policy. The robots.txt fetch stays here (external HTTP,
	// ADR-000954 D4); only the recorded result crosses to alt-data-hub.
	feedLinkDomainGw := feed_link_domain_gateway.NewFeedLinkDomainGateway(feedLinkGw)
	scrapingDomainUC := scraping_domain_usecase.NewScrapingDomainUsecaseWithFeedLinkDomain(
		scrapingDomainGw, robotsTxtGw, feedLinkDomainGw,
	)

	// og-image-backfill article fetch
	fetchArticleGw := fetch_article_gateway.NewFetchArticleGateway(hostRateLimiter, httpClient)

	// Image pipeline (ogp-image-warmer, og-image-backfill)
	imageProxyUC := newHarvesterImageProxyUsecase(cfg, feedLinkGw, imageProxyCacheGw)

	// Tag cloud read model. Both halves — the tag counts and the cooccurrence
	// edges — come from alt-data-hub since ADR-000954 Wave 3 batch 4 (catalog
	// §2.J), through the same gateway the backend uses. The octree layout and
	// the in-process cache stay here (D4).
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(
		datahub_gateway.NewTagGateway(dataHubClient), TagCloudCacheTTL)

	// outbox-worker targets: rag-orchestrator (REST only — the harvester never
	// speaks Connect-RPC, so it needs no mTLS leaf certificate) and
	// knowledge-sovereign.
	ragClient, err := rag_gateway.NewClientWithResponses(cfg.Rag.OrchestratorURL)
	if err != nil {
		panic("harvester: failed to create RAG client: " + err.Error())
	}
	ragAdapter := augur_adapter.NewAugurAdapter(ragClient)

	sovereignEnabled := LogSovereignWiringState("alt-harvester", cfg.Sovereign.URL, cfg.AppEnv)
	if !sovereignEnabled {
		panic("SOVEREIGN_URL is required for alt-harvester in every environment — " +
			"the outbox worker would mark rows PROCESSED while their knowledge events are dropped")
	}
	sovereignCli := sovereign_client.NewClient(cfg.Sovereign.URL, sovereignEnabled)

	return &HarvesterComponents{
		Config:                 cfg,
		DataHubClient:          dataHubClient,
		OutboxGateway:          outboxGw,
		OgImageGateway:         ogImageGw,
		ImageProxyCacheGateway: imageProxyCacheGw,
		FeedCollectorGateway:   feedCollectorGw,
		ScrapingDomainUsecase:  scrapingDomainUC,
		FetchArticleGateway:    fetchArticleGw,
		ImageProxyUsecase:      imageProxyUC,
		FetchTagCloudUsecase:   fetchTagCloudUC,
		RagIntegration:         ragAdapter,
		SovereignClient:        sovereignCli,
	}
}

// imageProxyFetchTimeout matches the dedicated image HTTP client the backend's
// image module uses.
const imageProxyFetchTimeout = 30 * time.Second

// imageProxyRateLimitInterval: CDN public images are fetched per job item, and
// 1 req/s/host avoids deadline-exceeded when several images share a host.
const imageProxyRateLimitInterval = 1 * time.Second

// TagCloudCacheTTL is the in-process cache window for the tag cloud read
// model. Exported because di/datahub needs the same window: the tag cloud is
// served to rag-orchestrator from cmd/datahub and rendered from cmd/harvester,
// and two constants would drift into two different answers to one question.
const TagCloudCacheTTL = 30 * time.Minute

func newHarvesterImageProxyUsecase(
	cfg *config.Config,
	feedLinkGw *datahub_gateway.FeedLinkGateway,
	cacheGw *datahub_gateway.ImageProxyCacheGateway,
) *image_proxy_usecase.ImageProxyUsecase {
	if !logImageProxyWiringState(cfg.ImageProxy.Enabled, cfg.ImageProxy.Secret != "") {
		return nil
	}

	imageHTTPClient := &http.Client{Timeout: imageProxyFetchTimeout}
	imageFetchGw := image_fetch_gateway.NewImageFetchGateway(imageHTTPClient)
	signer := image_proxy.NewSigner(cfg.ImageProxy.Secret)
	processingGw := image_proxy_gateway.NewProcessingGateway()
	dynamicDomainGw := image_proxy_gateway.NewDynamicDomainGateway(feedLinkGw)

	return image_proxy_usecase.NewImageProxyUsecase(
		imageFetchGw,
		processingGw,
		cacheGw,
		signer,
		dynamicDomainGw,
		rate_limiter.NewHostRateLimiter(imageProxyRateLimitInterval),
		cfg.ImageProxy.MaxWidth,
		cfg.ImageProxy.WebPQuality,
		cfg.ImageProxy.CacheTTLMin,
	)
}

// LogSovereignWiringState is shared by all three composition roots, so it
// records which binary emitted it — three processes now produce this line and
// they are otherwise indistinguishable in the log stream. Exported for
// di/datahub, which became a package of its own when the database left the
// other two binaries (ADR-000954 Wave 3 batch 6).
func LogSovereignWiringState(binary, sovereignURL, appEnv string) bool {
	enabled := sovereignURL != ""
	if enabled {
		slog.Info("sovereign_enabled", "binary", binary, "base_url", sovereignURL)
		return true
	}
	slog.Warn("sovereign_disabled", "binary", binary,
		"reason", "SOVEREIGN_URL unset; all Knowledge Home mutations will no-op")
	if appEnv == "production" {
		panic("SOVEREIGN_URL is required when APP_ENV=production — refusing to start with Knowledge Home silently disabled")
	}
	return false
}

// harvesterFeedCollectorGateway is the hourly collector's whole view of
// alt-data-hub: the poll work list, the upsert of what polling produced, and
// the poll-health state machine.
//
// Three embedded gateways rather than one wide one, because they are three
// tables with three different invariants — and because embedding keeps
// job.FeedCollectorStore satisfied by method promotion without this struct
// restating a signature that would then have two places to drift.
type harvesterFeedCollectorGateway struct {
	*datahub_gateway.FeedLinkGateway
	*datahub_gateway.FeedGateway
	*datahub_gateway.FeedLinkAvailabilityGateway
}
