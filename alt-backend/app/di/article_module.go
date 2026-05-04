package di

import (
	"alt/gateway/archive_article_gateway"
	"alt/gateway/article_content_cache_gateway"
	"alt/gateway/article_gateway"
	"alt/gateway/article_summary_gateway"
	"alt/gateway/cached_article_tags_gateway"
	"alt/gateway/fetch_article_gateway"
	"alt/gateway/fetch_article_tags_gateway"
	"alt/gateway/fetch_articles_by_tag_gateway"
	"alt/gateway/fetch_recent_articles_gateway"
	"alt/gateway/fetch_tag_cloud_gateway"
	"alt/gateway/internal_article_gateway"
	"alt/gateway/latest_article_gateway"
	"alt/gateway/scraping_policy_gateway"
	"alt/port/rag_integration_port"
	"alt/usecase/archive_article_usecase"
	"alt/usecase/fetch_article_summary_usecase"
	"alt/usecase/fetch_article_tags_usecase"
	"alt/usecase/fetch_article_usecase"
	"alt/usecase/fetch_articles_by_tag_usecase"
	"alt/usecase/fetch_articles_usecase"
	"alt/usecase/fetch_latest_article_usecase"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/usecase/fetch_tag_cloud_usecase"
	"alt/usecase/get_article_source_url_usecase"
	"alt/usecase/search_article_usecase"
	"alt/usecase/stream_article_tags_usecase"
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
	FetchRecentArticlesUsecase *fetch_recent_articles_usecase.FetchRecentArticlesUsecase
	ArticleSearchUsecase       *search_article_usecase.SearchArticleUsecase
	BatchArticleFetcher        *batch_article_fetcher.BatchArticleFetcher
	FetchTagCloudUsecase       *fetch_tag_cloud_usecase.FetchTagCloudUsecase
	GetArticleSourceURLUsecase *get_article_source_url_usecase.GetArticleSourceURLUsecase

	// Gateways exposed for cross-module wiring
	InternalArticleGateway  *internal_article_gateway.Gateway
	FetchArticleTagsGateway *fetch_article_tags_gateway.FetchArticleTagsGateway
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
	fetchArticleUC := fetch_article_usecase.NewArticleUsecaseWithScrapingPolicy(
		fetchArticleGw, infra.RobotsTxtGateway, altDB, ragAdapter, scrapingPolicyGw,
	)

	// Batch article fetcher for efficient multi-URL fetching with domain-based rate limiting
	batchFetcher := batch_article_fetcher.NewBatchArticleFetcher(infra.RateLimiter, infra.HTTPClient)

	// Fetch articles with cursor
	fetchArticlesGw := article_gateway.NewFetchArticlesGateway(pool)
	articleContentCacheGw := article_content_cache_gateway.NewGateway(altDB)
	fetchArticlesCursorUC := fetch_articles_usecase.NewFetchArticlesCursorUsecaseWithCache(fetchArticlesGw, articleContentCacheGw)

	// Fetch recent articles (for rag-orchestrator temporal topics)
	fetchRecentArticlesGw := fetch_recent_articles_gateway.NewFetchRecentArticlesGateway(pool)
	fetchRecentArticlesUC := fetch_recent_articles_usecase.NewFetchRecentArticlesUsecase(fetchRecentArticlesGw)

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

	// Internal article API gateway (for BackendInternalService)
	internalArticleGw := internal_article_gateway.NewGateway(altDB)

	// GetArticleSourceURL: tenant-scoped read-side lookup for the Knowledge
	// Loop ACT workspace's Open recovery affordance. Reuses the
	// ArticleURLLookupGateway already defined in knowledge_module.go.
	articleURLLookupGw := article_gateway.NewArticleURLLookupGateway(infra.Pool)
	getArticleSourceURLUC := get_article_source_url_usecase.NewGetArticleSourceURLUsecase(articleURLLookupGw)

	return &ArticleModule{
		ArticleUsecase:             fetchArticleUC,
		ArchiveArticleUsecase:      archiveArticleUC,
		FetchArticlesCursorUsecase: fetchArticlesCursorUC,
		FetchArticleTagsUsecase:    fetchArticleTagsUC,
		FetchArticlesByTagUsecase:  fetchArticlesByTagUC,
		FetchLatestArticleUsecase:  fetchLatestArticleUC,
		FetchArticleSummaryUsecase: fetchArticleSummaryUC,
		StreamArticleTagsUsecase:   streamArticleTagsUC,
		FetchRecentArticlesUsecase: fetchRecentArticlesUC,
		ArticleSearchUsecase:       articleSearchUC,
		BatchArticleFetcher:        batchFetcher,
		FetchTagCloudUsecase:       fetchTagCloudUC,
		GetArticleSourceURLUsecase: getArticleSourceURLUC,

		InternalArticleGateway:  internalArticleGw,
		FetchArticleTagsGateway: fetchArticleTagsGw,
	}
}
