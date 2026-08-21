package di

import (
	"alt/config"
	"alt/orchestrator/connect/v2/articles"
	"alt/orchestrator/driver/preprocessor_client"
	"alt/orchestrator/gateway/archive_article_gateway"
	"alt/orchestrator/gateway/article_content_cache_gateway"
	"alt/orchestrator/gateway/article_repository_gateway"
	"alt/orchestrator/gateway/article_summary_gateway"
	"alt/orchestrator/gateway/cached_article_tags_gateway"
	"alt/orchestrator/gateway/fetch_article_gateway"
	"alt/orchestrator/gateway/fetch_article_tags_gateway"
	"alt/orchestrator/gateway/latest_article_gateway"
	"alt/orchestrator/gateway/preprocessor_summarize_gateway"
	"alt/orchestrator/gateway/scraping_policy_gateway"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/usecase/archive_article_usecase"
	"alt/orchestrator/usecase/fetch_article_summaries_usecase"
	"alt/orchestrator/usecase/fetch_article_summary_usecase"
	"alt/orchestrator/usecase/fetch_article_tags_usecase"
	"alt/orchestrator/usecase/fetch_article_usecase"
	"alt/orchestrator/usecase/fetch_articles_usecase"
	"alt/orchestrator/usecase/fetch_latest_article_usecase"
	"alt/orchestrator/usecase/get_article_source_url_usecase"
	"alt/orchestrator/usecase/search_article_usecase"
	"alt/orchestrator/usecase/stream_article_tags_usecase"
	"alt/orchestrator/usecase/summarize_article_usecase"
	"alt/shared/gateway/datahub_gateway"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils/batch_article_fetcher"
	"alt/utils/rate_limiter"
	"fmt"
	"log/slog"
	"time"
)

// ArticleModule holds all article-domain components.
type ArticleModule struct {
	// Usecases
	ArticleUsecase             fetch_article_usecase.ArticleUsecase
	ArchiveArticleUsecase      *archive_article_usecase.ArchiveArticleUsecase
	FetchArticlesCursorUsecase *fetch_articles_usecase.FetchArticlesCursorUsecase
	FetchArticleTagsUsecase    *fetch_article_tags_usecase.FetchArticleTagsUsecase
	FetchArticlesByTagUsecase  *fetch_articles_by_tag_usecase.FetchArticlesByTagUsecase
	FetchLatestArticleUsecase  *fetch_latest_article_usecase.FetchLatestArticleUsecase
	FetchArticleSummaryUsecase *fetch_article_summary_usecase.FetchArticleSummaryUsecase
	StreamArticleTagsUsecase   *stream_article_tags_usecase.StreamArticleTagsUsecase
	ArticleSearchUsecase       *search_article_usecase.SearchArticleUsecase
	BatchArticleFetcher        *batch_article_fetcher.BatchArticleFetcher
	FetchTagCloudUsecase       *fetch_tag_cloud_usecase.FetchTagCloudUsecase
	GetArticleSourceURLUsecase *get_article_source_url_usecase.GetArticleSourceURLUsecase

	// Legacy REST v1 summarize endpoints (POST /v1/feeds/summarize,
	// /summarize/queue, GET /summarize/status/:job_id, POST /fetch/summary).
	SummarizeArticleUsecase      *summarize_article_usecase.Usecase
	FetchArticleSummariesUsecase *fetch_article_summaries_usecase.Usecase
	PreProcessorSummarizeGateway *preprocessor_summarize_gateway.Gateway

	// Article-content prefetch (BatchPrefetchArticleContent). A second
	// ArticleUsecase over the same repository, robots and scraping-policy
	// ports as ArticleUsecase, differing only in its fetch gateway — see
	// newArticleModule for why the scraping-policy instance must be shared and
	// the fetch gateway must not be.
	PrefetchArticleUsecase fetch_article_usecase.ArticleUsecase
	PrefetchArticleProbe   articles.StoredArticleProbe
	PrefetchHostSlots      articles.HostSlotGate
	PrefetchWiring         articles.ArticlePrefetchWiring

	// Gateways exposed for cross-module wiring
	FetchArticleTagsGateway *fetch_article_tags_gateway.FetchArticleTagsGateway
	FetchArticleGateway     *fetch_article_gateway.FetchArticleGateway
}

// logInteractiveSlotWait states, once at startup, how long a user-facing
// article fetch will queue for a host. Zero is a legitimate setting — it makes
// the interactive path queue like a background job — but it is the setting
// that reintroduces the priority inversion, so it must never be reachable by
// silence (CLAUDE.md rules 8 and 9).
func logInteractiveSlotWait(budget time.Duration) {
	if budget <= 0 {
		slog.Warn("fetch_article_gateway.interactive_slot_wait_disabled",
			"reason", "RATE_LIMIT_INTERACTIVE_SLOT_WAIT is zero",
			"impact", "a user-facing article fetch queues for its turn at a host until its own deadline expires, behind any background job holding that host")
		return
	}

	slog.Info("fetch_article_gateway.interactive_slot_wait_enabled", "budget", budget)
}

// newArticlePrefetchWiring decides, once, whether this binary warms article
// bodies on request, and with what budget.
//
// The decision is a declaration rather than an inference from a nil pointer
// (ADR-000966 §2): every port a prefetch uses is shared with the interactive
// read path, so "the ports are there" says nothing about whether warming is
// meant to happen. RATE_LIMIT_PREFETCH_SLOT_WAIT=off is the only way to turn
// it off, and it is a value an operator writes rather than one they omit.
func newArticlePrefetchWiring(rl config.RateLimitConfig) (articles.ArticlePrefetchWiring, error) {
	budget, enabled, err := rl.PrefetchSlotWaitSetting()
	if err != nil {
		return articles.ArticlePrefetchWiring{}, err
	}

	if !enabled {
		return articles.ArticlePrefetchWiring{
			Enabled:        false,
			DisabledReason: "RATE_LIMIT_PREFETCH_SLOT_WAIT=off",
		}, nil
	}

	return articles.ArticlePrefetchWiring{Enabled: true, SlotWait: budget}, nil
}

// logArticlePrefetchWiring states, once at startup, which of the two states
// this process is in. Without it, "prefetch is deliberately off" and "prefetch
// was never wired" are the same silence — the failure ADR-000928 is about, and
// the reason CLAUDE.md rule 8 asks for a loud line either way.
func logArticlePrefetchWiring(wiring articles.ArticlePrefetchWiring) {
	if !wiring.Enabled {
		slog.Warn("article_prefetch.disabled",
			"reason", wiring.DisabledReason,
			"impact", "BatchPrefetchArticleContent answers FAILED_PRECONDITION; article bodies are fetched only when a reader opens one")
		return
	}

	slog.Info("article_prefetch.enabled",
		"slot_wait", wiring.SlotWait.String(),
		"namespace", rate_limiter.NamespaceExternalAPI,
		"max_urls_per_call", 5,
		"one_url_per_host", true,
		"note", "warms take a host's turn from the same limiter and namespace as interactive fetches, and give it up after slot_wait rather than queueing for it")
}

func newArticleModule(infra *InfraModule, feed *FeedModule, ragAdapter rag_integration_port.RagIntegrationPort) *ArticleModule {
	// No alt_db handle. The two that were left here through batch 5 — the Tag
	// Trail's paged read and RecallRailUsecase's article fallback — became
	// procedures of their own in ADR-000954 Wave 3 batch 6, which is what let
	// cmd/backend stop opening a pool at all.

	// Fetch article gateway / usecase. These are the fetches a user is waiting
	// on, so they bound how long they queue for a host and hand back a
	// retry-after instead of spending the request deadline behind the
	// harvester's collector. cmd/harvester and the batch fetcher build their
	// own gateways and keep queueing — they have nobody waiting.
	slotWait := infra.Config.RateLimit.InteractiveSlotWait
	logInteractiveSlotWait(slotWait)
	fetchArticleGw := fetch_article_gateway.NewInteractiveFetchArticleGateway(
		infra.RateLimiter, infra.HTTPClient, slotWait)
	archiveArticleGw := archive_article_gateway.NewArchiveArticleGateway(infra.ArticleStoreGateway)
	archiveArticleUC := archive_article_usecase.NewArchiveArticleUsecase(fetchArticleGw, archiveArticleGw)

	// Wire ScrapingPolicyGateway into ArticleUsecase (uses cached robots.txt from scraping_domains)
	scrapingPolicyGw := scraping_policy_gateway.NewScrapingPolicyGateway(feed.ScrapingDomainGateway)

	// ArticleRepository is served entirely by alt-data-hub since ADR-000954
	// Wave 3 batch 2: the article read and write (§2.B / §2.C) joined
	// article_heads (§2.D) and declined_domains (§2.L). The usecase's
	// interface is unchanged — see article_repository_gateway for why the
	// three gateways stay separate.
	articleRepoGw := article_repository_gateway.New(infra.ArticleStoreGateway, infra.OgImageGateway, infra.DeclinedDomainGateway)
	fetchArticleUC := fetch_article_usecase.NewArticleUsecaseWithScrapingPolicy(
		fetchArticleGw, infra.RobotsTxtGateway, articleRepoGw, ragAdapter, scrapingPolicyGw,
	)

	// The third fetch class: article-content prefetch. Same repository, robots
	// and scraping-policy ports as the interactive usecase above, a different
	// fetch gateway.
	//
	// Sharing scrapingPolicyGw is load bearing, not incidental. Crawl-delay
	// state lives in that gateway instance (lastRequestTime), and a granted
	// CanFetchArticle *reserves* the window rather than merely reporting on
	// it. A second instance would give the reader and the warmer one grant
	// each inside a single crawl delay — two requests where the publisher was
	// promised one.
	//
	// Not sharing the fetch gateway is equally load bearing. The prefetch
	// gateway holds no rate limiter because the handler takes the host's turn
	// itself, before the usecase reaches the policy gate; see
	// NewPrefetchFetchArticleGateway and BatchPrefetchArticleContent for why
	// that order is the difference between a warm that helps the reader and
	// one that starves them.
	prefetchWiring, wiringErr := newArticlePrefetchWiring(infra.Config.RateLimit)
	if wiringErr != nil {
		// config.validateRateLimitConfig refuses these values at Load(), so
		// arriving here means this root was handed a config that never went
		// through it. That is a wiring bug, and it must stop the process
		// rather than pick a budget on the operator's behalf.
		panic(fmt.Sprintf("article prefetch wiring: %v", wiringErr))
	}
	logArticlePrefetchWiring(prefetchWiring)

	var prefetchArticleUC fetch_article_usecase.ArticleUsecase
	if prefetchWiring.Enabled {
		prefetchArticleUC = fetch_article_usecase.NewArticleUsecaseWithScrapingPolicy(
			fetch_article_gateway.NewPrefetchFetchArticleGateway(),
			infra.RobotsTxtGateway, articleRepoGw, ragAdapter, scrapingPolicyGw,
		)
	}

	// Batch article fetcher for efficient multi-URL fetching with domain-based rate limiting
	batchFetcher := batch_article_fetcher.NewBatchArticleFetcher(infra.RateLimiter, infra.HTTPClient)

	// Fetch articles with cursor (catalog §2.C W3-C4 / W3-C5). The read-through
	// cache stays here: a TTL is flow orchestration, which ADR-000954 D4 keeps
	// on the calling side.
	fetchArticlesGw := infra.ArticleCursorGateway
	articleContentCacheGw := article_content_cache_gateway.NewGateway(infra.ArticleBatchGateway)
	fetchArticlesCursorUC := fetch_articles_usecase.NewFetchArticlesCursorUsecaseWithCache(fetchArticlesGw, articleContentCacheGw)

	// Recent articles for rag-orchestrator's temporal topics moved to
	// cmd/datahub with the /v1/internal REST route that serves them.

	// Article search (Meilisearch-based via search-indexer)
	articleSearchUC := search_article_usecase.NewSearchArticleUsecase(infra.SearchIndexerDriver)

	// Articles by tag (Tag Trail feature). Served by alt-data-hub since
	// ADR-000954 Wave 3 batch 6 (catalog §2.J): the Wave 2 FetchArticlesByTag
	// procedure could not express this caller's paging, so the Trail got
	// ListArticlesByTagID / ListArticlesByTagName and TagGateway grew the two
	// halves of the port.
	fetchArticlesByTagUC := fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(infra.TagGateway)

	// Tag cloud (Tag Verse feature). Both halves of the port — the counts and
	// the cooccurrence edges — now take the same route (catalog §2.J W3-J3);
	// alt-backend used to serve FetchTagCloud as an RPC to rag-orchestrator
	// while reading the same table directly for itself. The octree layout and
	// the 30-minute cache stay in the usecase (ADR-000954 D4).
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(infra.TagGateway, 30*time.Minute)

	// Article tags (Tag Trail feature, with mq-hub for on-the-fly tag generation ADR-168).
	// One source since batch 4: the tag read, the tag write-back and the
	// article body all arrive over the same connection (catalog §2.J / W2-13).
	fetchArticleTagsConfig := fetch_article_tags_gateway.DefaultConfig()
	fetchArticleTagsGw := fetch_article_tags_gateway.NewFetchArticleTagsGatewayWithMQHub(
		infra.TagGateway, infra.ArticleStoreGateway, infra.MQHubClient, fetchArticleTagsConfig,
	)
	fetchArticleTagsUC := fetch_article_tags_usecase.NewFetchArticleTagsUsecase(fetchArticleTagsGw)

	// Article summary
	articleSummaryGw := article_summary_gateway.NewGateway(infra.FeedGateway)
	fetchArticleSummaryUC := fetch_article_summary_usecase.NewFetchArticleSummaryUsecase(articleSummaryGw)

	// Latest article (FetchRandomFeed)
	latestArticleGw := latest_article_gateway.NewGateway(infra.LatestArticleGateway)
	fetchLatestArticleUC := fetch_latest_article_usecase.NewFetchLatestArticleUsecase(latestArticleGw)

	// Stream article tags (cached check + on-the-fly generation)
	cachedArticleTagsGw := cached_article_tags_gateway.NewGateway(infra.TagGateway)
	streamArticleTagsUC := stream_article_tags_usecase.NewStreamArticleTagsUsecase(
		cachedArticleTagsGw, fetchArticleTagsGw,
	)

	// GetArticleSourceURL: tenant-scoped read-side lookup for the Knowledge
	// Loop ACT workspace's Open recovery affordance. Reuses the
	// ArticleURLLookupGateway already defined in knowledge_module.go.
	getArticleSourceURLUC := get_article_source_url_usecase.NewGetArticleSourceURLUsecase(infra.ArticleURLLookupGateway)

	// Legacy REST v1 summarize endpoints. Single driver-layer pre-processor
	// HTTP client, wrapped by a gateway satisfying preprocessor_summarize_port,
	// consolidating what was previously ~600 lines duplicated across
	// rest/utils.go, rest/rest_feeds/utils.go, and
	// rest/rest_feeds/summarization/helpers.go.
	preprocessorClient := preprocessor_client.NewClient(infra.Config.PreProcessor.URL)
	preprocessorSummarizeGw := preprocessor_summarize_gateway.NewGateway(preprocessorClient)
	//
	// One source since ADR-000954 Wave 3 batch 5. The article read and write
	// arrived in batch 2 (catalog §2.B / §2.C), the article-summary read in
	// batch 3 (§2.H W3-H8), and the summary *write* — the last direct alt_db
	// call alt-backend held — in batch 5.
	summarizeRepo := summarize_article_repository{
		ArticleStoreGateway: infra.ArticleStoreGateway,
		FeedGateway:         infra.FeedGateway,
	}
	summarizeArticleUC := summarize_article_usecase.NewUsecase(summarizeRepo, preprocessorSummarizeGw, fetchArticleGw)
	fetchArticleSummariesUC := fetch_article_summaries_usecase.NewUsecase(summarizeRepo, batchFetcher, summarizeArticleUC)

	return &ArticleModule{
		ArticleUsecase:             fetchArticleUC,
		ArchiveArticleUsecase:      archiveArticleUC,
		FetchArticlesCursorUsecase: fetchArticlesCursorUC,
		FetchArticleTagsUsecase:    fetchArticleTagsUC,
		FetchArticlesByTagUsecase:  fetchArticlesByTagUC,
		FetchLatestArticleUsecase:  fetchLatestArticleUC,
		FetchArticleSummaryUsecase: fetchArticleSummaryUC,
		StreamArticleTagsUsecase:   streamArticleTagsUC,
		ArticleSearchUsecase:       articleSearchUC,
		BatchArticleFetcher:        batchFetcher,
		FetchTagCloudUsecase:       fetchTagCloudUC,
		GetArticleSourceURLUsecase: getArticleSourceURLUC,

		PrefetchArticleUsecase: prefetchArticleUC,
		PrefetchArticleProbe:   articleRepoGw,
		PrefetchHostSlots:      infra.RateLimiter,
		PrefetchWiring:         prefetchWiring,

		SummarizeArticleUsecase:      summarizeArticleUC,
		FetchArticleSummariesUsecase: fetchArticleSummariesUC,
		PreProcessorSummarizeGateway: preprocessorSummarizeGw,

		FetchArticleTagsGateway: fetchArticleTagsGw,
		FetchArticleGateway:     fetchArticleGw,
	}
}

// summarize_article_repository satisfies the local ArticleRepository
// interfaces of summarize_article_usecase and fetch_article_summaries_usecase
// out of two alt-data-hub gateways.
//
// The article read and write are catalog §2.B / §2.C (batch 2); the
// article-summary read is §2.H W3-H8 (batch 3); the summary write is the seam
// batch 5 closed. Embedding both gateways is what lets the usecases keep the
// interfaces they declared: Go promotes each method from whichever embedded
// type has it, and the two sets are disjoint.
//
// The *alt_db.AltDBRepository that used to sit here as a third embed is gone,
// and with it the one thing that made this type fragile. While it was present,
// the summary read resolved to FeedGateway rather than to the driver only
// because Go promotes the shallower method — depth one against depth two
// through FeedRepository — so flattening the embed would silently have sent
// that read back to the database. There is now no second candidate for any
// method here.
type summarize_article_repository struct {
	*datahub_gateway.ArticleStoreGateway
	*datahub_gateway.FeedGateway
}
