package di

import (
	"log/slog"
	"net/http"
	"time"

	"alt/adapter/augur_adapter"
	"alt/config"
	"alt/orchestrator/gateway/feed_link_domain_gateway"
	"alt/orchestrator/gateway/fetch_article_gateway"
	"alt/orchestrator/gateway/image_fetch_gateway"
	"alt/orchestrator/gateway/image_proxy_gateway"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/gateway/robots_txt_gateway"
	"alt/orchestrator/gateway/scraping_domain_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/shared/driver/alt_db"
	"alt/shared/driver/sovereign_client"
	"alt/shared/gateway/fetch_tag_cloud_gateway"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils"
	"alt/utils/image_proxy"
	"alt/utils/rate_limiter"

	"github.com/jackc/pgx/v5/pgxpool"
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

	// Shared driver
	AltDBRepository *alt_db.AltDBRepository

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
func NewHarvesterComponents(pool *pgxpool.Pool, cfg *config.Config) *HarvesterComponents {
	altDB := alt_db.NewAltDBRepository(pool)

	// Shared external-fetch primitives. CLAUDE.md rule 2: this rate limiter is
	// process-local, so the harvester's effective rate against a publisher is
	// independent of the backend's — see the split ADR's R1 note.
	rateLimitInterval := cfg.RateLimit.ExternalAPIInterval
	hostRateLimiter := rate_limiter.NewHostRateLimiter(rateLimitInterval, cfg.RateLimit.ExternalAPIBurst)
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()
	robotsTxtGw := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// daily-scraping-policy
	scrapingDomainGw := scraping_domain_gateway.NewScrapingDomainGateway(altDB)
	feedLinkDomainGw := feed_link_domain_gateway.NewFeedLinkDomainGateway(altDB)
	scrapingDomainUC := scraping_domain_usecase.NewScrapingDomainUsecaseWithFeedLinkDomain(
		scrapingDomainGw, robotsTxtGw, feedLinkDomainGw,
	)

	// og-image-backfill article fetch
	fetchArticleGw := fetch_article_gateway.NewFetchArticleGateway(hostRateLimiter, httpClient)

	// Image pipeline (ogp-image-warmer, og-image-backfill)
	imageProxyUC := newHarvesterImageProxyUsecase(cfg, altDB)

	// Tag cloud read model
	fetchTagCloudGw := fetch_tag_cloud_gateway.NewFetchTagCloudGateway(altDB)
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(fetchTagCloudGw, tagCloudCacheTTL)

	// outbox-worker targets: rag-orchestrator (REST only — the harvester never
	// speaks Connect-RPC, so it needs no mTLS leaf certificate) and
	// knowledge-sovereign.
	ragClient, err := rag_gateway.NewClientWithResponses(cfg.Rag.OrchestratorURL)
	if err != nil {
		panic("harvester: failed to create RAG client: " + err.Error())
	}
	ragAdapter := augur_adapter.NewAugurAdapter(ragClient)

	sovereignEnabled := logSovereignWiringState("alt-harvester", cfg.Sovereign.URL, cfg.AppEnv)
	if !sovereignEnabled {
		panic("SOVEREIGN_URL is required for alt-harvester in every environment — " +
			"the outbox worker would mark rows PROCESSED while their knowledge events are dropped")
	}
	sovereignCli := sovereign_client.NewClient(cfg.Sovereign.URL, sovereignEnabled)

	return &HarvesterComponents{
		Config:                cfg,
		AltDBRepository:       altDB,
		ScrapingDomainUsecase: scrapingDomainUC,
		FetchArticleGateway:   fetchArticleGw,
		ImageProxyUsecase:     imageProxyUC,
		FetchTagCloudUsecase:  fetchTagCloudUC,
		RagIntegration:        ragAdapter,
		SovereignClient:       sovereignCli,
	}
}

// imageProxyFetchTimeout matches the dedicated image HTTP client the backend's
// image module uses.
const imageProxyFetchTimeout = 30 * time.Second

// imageProxyRateLimitInterval: CDN public images are fetched per job item, and
// 1 req/s/host avoids deadline-exceeded when several images share a host.
const imageProxyRateLimitInterval = 1 * time.Second

// tagCloudCacheTTL is the in-process cache window for the tag cloud read model.
const tagCloudCacheTTL = 30 * time.Minute

func newHarvesterImageProxyUsecase(cfg *config.Config, altDB *alt_db.AltDBRepository) *image_proxy_usecase.ImageProxyUsecase {
	if !logImageProxyWiringState(cfg.ImageProxy.Enabled, cfg.ImageProxy.Secret != "") {
		return nil
	}

	imageHTTPClient := &http.Client{Timeout: imageProxyFetchTimeout}
	imageFetchGw := image_fetch_gateway.NewImageFetchGateway(imageHTTPClient)
	signer := image_proxy.NewSigner(cfg.ImageProxy.Secret)
	cacheGw := image_proxy_gateway.NewCacheGateway(altDB)
	processingGw := image_proxy_gateway.NewProcessingGateway()
	dynamicDomainGw := image_proxy_gateway.NewDynamicDomainGateway(altDB)

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

// logSovereignWiringState is shared by all three composition roots, so it
// records which binary emitted it — three processes now produce this line and
// they are otherwise indistinguishable in the log stream.
func logSovereignWiringState(binary, sovereignURL, appEnv string) bool {
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
