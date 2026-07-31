package di

import (
	"alt/orchestrator/driver/preprocessor_client"
	"alt/orchestrator/gateway/archive_article_gateway"
	"alt/orchestrator/gateway/article_content_cache_gateway"
	"alt/orchestrator/gateway/article_gateway"
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
	"alt/shared/gateway/fetch_articles_by_tag_gateway"
	"alt/shared/gateway/fetch_tag_cloud_gateway"
	"alt/shared/gateway/internal_article_gateway"
	"alt/shared/usecase/fetch_articles_by_tag_usecase"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils/batch_article_fetcher"
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

	// Gateways exposed for cross-module wiring
	InternalArticleGateway  *internal_article_gateway.Gateway
	FetchArticleTagsGateway *fetch_article_tags_gateway.FetchArticleTagsGateway
	FetchArticleGateway     *fetch_article_gateway.FetchArticleGateway
}

func newArticleModule(infra *InfraModule, feed *FeedModule, ragAdapter rag_integration_port.RagIntegrationPort) *ArticleModule {
	pool := infra.Pool
	altDB := infra.AltDBRepository

	// Fetch article gateway / usecase
	fetchArticleGw := fetch_article_gateway.NewFetchArticleGateway(infra.RateLimiter, infra.HTTPClient)
	archiveArticleGw := archive_article_gateway.NewArchiveArticleGateway(altDB)
	archiveArticleUC := archive_article_usecase.NewArchiveArticleUsecase(fetchArticleGw, archiveArticleGw)

	// Wire ScrapingPolicyGateway into ArticleUsecase (uses cached robots.txt from scraping_domains)
	scrapingPolicyGw := scraping_policy_gateway.NewScrapingPolicyGateway(feed.ScrapingDomainGateway)

	// ArticleRepository is served from two sources during ADR-000954 Wave 3:
	// article_heads and declined_domains come from alt-data-hub (batch 1,
	// catalog §2.D / §2.L), the article read and write are still direct
	// (§2.B / §2.C, later batches). See article_repository_gateway — the
	// usecase's interface is unchanged.
	articleRepoGw := article_repository_gateway.New(altDB, infra.OgImageGateway, infra.DeclinedDomainGateway)
	fetchArticleUC := fetch_article_usecase.NewArticleUsecaseWithScrapingPolicy(
		fetchArticleGw, infra.RobotsTxtGateway, articleRepoGw, ragAdapter, scrapingPolicyGw,
	)

	// Batch article fetcher for efficient multi-URL fetching with domain-based rate limiting
	batchFetcher := batch_article_fetcher.NewBatchArticleFetcher(infra.RateLimiter, infra.HTTPClient)

	// Fetch articles with cursor
	fetchArticlesGw := article_gateway.NewFetchArticlesGateway(pool)
	articleContentCacheGw := article_content_cache_gateway.NewGateway(altDB)
	fetchArticlesCursorUC := fetch_articles_usecase.NewFetchArticlesCursorUsecaseWithCache(fetchArticlesGw, articleContentCacheGw)

	// Recent articles for rag-orchestrator's temporal topics moved to
	// cmd/datahub with the /v1/internal REST route that serves them.

	// Article search (Meilisearch-based via search-indexer)
	articleSearchUC := search_article_usecase.NewSearchArticleUsecase(infra.SearchIndexerDriver)

	// Articles by tag (Tag Trail feature)
	fetchArticlesByTagGw := fetch_articles_by_tag_gateway.NewFetchArticlesByTagGateway(altDB)
	fetchArticlesByTagUC := fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(fetchArticlesByTagGw)

	// Tag cloud (Tag Verse feature)
	fetchTagCloudGw := fetch_tag_cloud_gateway.NewFetchTagCloudGateway(altDB)
	fetchTagCloudUC := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(fetchTagCloudGw, 30*time.Minute)

	// Article tags (Tag Trail feature, with mq-hub for on-the-fly tag generation ADR-168)
	fetchArticleTagsConfig := fetch_article_tags_gateway.DefaultConfig()
	fetchArticleTagsGw := fetch_article_tags_gateway.NewFetchArticleTagsGatewayWithMQHub(
		altDB, infra.MQHubClient, fetchArticleTagsConfig,
	)
	fetchArticleTagsUC := fetch_article_tags_usecase.NewFetchArticleTagsUsecase(fetchArticleTagsGw)

	// Article summary
	articleSummaryGw := article_summary_gateway.NewGateway(altDB)
	fetchArticleSummaryUC := fetch_article_summary_usecase.NewFetchArticleSummaryUsecase(articleSummaryGw)

	// Latest article (FetchRandomFeed)
	latestArticleGw := latest_article_gateway.NewGateway(altDB)
	fetchLatestArticleUC := fetch_latest_article_usecase.NewFetchLatestArticleUsecase(latestArticleGw)

	// Stream article tags (cached check + on-the-fly generation)
	cachedArticleTagsGw := cached_article_tags_gateway.NewGateway(altDB)
	streamArticleTagsUC := stream_article_tags_usecase.NewStreamArticleTagsUsecase(
		cachedArticleTagsGw, fetchArticleTagsGw,
	)

	// Internal article API gateway (for DataHubService)
	internalArticleGw := internal_article_gateway.NewGateway(altDB)

	// GetArticleSourceURL: tenant-scoped read-side lookup for the Knowledge
	// Loop ACT workspace's Open recovery affordance. Reuses the
	// ArticleURLLookupGateway already defined in knowledge_module.go.
	articleURLLookupGw := article_gateway.NewArticleURLLookupGateway(infra.Pool)
	getArticleSourceURLUC := get_article_source_url_usecase.NewGetArticleSourceURLUsecase(articleURLLookupGw)

	// Legacy REST v1 summarize endpoints. Single driver-layer pre-processor
	// HTTP client, wrapped by a gateway satisfying preprocessor_summarize_port,
	// consolidating what was previously ~600 lines duplicated across
	// rest/utils.go, rest/rest_feeds/utils.go, and
	// rest/rest_feeds/summarization/helpers.go.
	preprocessorClient := preprocessor_client.NewClient(infra.Config.PreProcessor.URL)
	preprocessorSummarizeGw := preprocessor_summarize_gateway.NewGateway(preprocessorClient)
	summarizeArticleUC := summarize_article_usecase.NewUsecase(altDB, preprocessorSummarizeGw, fetchArticleGw)
	fetchArticleSummariesUC := fetch_article_summaries_usecase.NewUsecase(altDB, batchFetcher, summarizeArticleUC)

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

		SummarizeArticleUsecase:      summarizeArticleUC,
		FetchArticleSummariesUsecase: fetchArticleSummariesUC,
		PreProcessorSummarizeGateway: preprocessorSummarizeGw,

		InternalArticleGateway:  internalArticleGw,
		FetchArticleTagsGateway: fetchArticleTagsGw,
		FetchArticleGateway:     fetchArticleGw,
	}
}
