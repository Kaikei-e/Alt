package di

import (
	"log/slog"

	"alt/adapter/augur_adapter"
	"alt/config"
	"alt/driver/alt_db"
	"alt/driver/csrf_token_driver"
	"alt/driver/kratos_client"
	"alt/driver/mqhub_connect"
	"alt/driver/recap_job_driver"
	"alt/driver/search_indexer_connect"
	"alt/gateway/archive_article_gateway"
	"alt/gateway/article_content_cache_gateway"
	"alt/gateway/article_gateway"
	"alt/gateway/article_summary_gateway"
	"alt/gateway/cached_article_tags_gateway"
	"alt/gateway/config_gateway"
	"alt/gateway/csrf_token_gateway"
	"alt/gateway/dashboard_gateway"
	"alt/gateway/error_handler_gateway"
	"alt/gateway/event_publisher_gateway"
	"alt/gateway/feature_flag_gateway"
	"alt/gateway/feed_link_domain_gateway"
	"alt/gateway/feed_link_gateway"
	"alt/gateway/feed_page_cache_gateway"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_link_gateway"
	"alt/gateway/feed_url_to_id_gateway"
	"alt/gateway/fetch_article_gateway"
	"alt/gateway/fetch_article_tags_gateway"
	"alt/gateway/fetch_articles_by_tag_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/fetch_feed_tags_gateway"
	"alt/gateway/fetch_inoreader_summary_gateway"
	"alt/gateway/fetch_random_subscription_gateway"
	"alt/gateway/fetch_recent_articles_gateway"
	"alt/gateway/fetch_tag_cloud_gateway"
	"alt/gateway/image_fetch_gateway"
	"alt/gateway/image_proxy_gateway"
	"alt/gateway/internal_article_gateway"
	"alt/gateway/knowledge_backfill_gateway"
	"alt/gateway/knowledge_event_gateway"
	"alt/gateway/knowledge_home_gateway"
	"alt/gateway/knowledge_sovereign_gateway"
	"alt/gateway/knowledge_lens_gateway"
	"alt/gateway/knowledge_projection_gateway"
	"alt/gateway/knowledge_projection_version_gateway"
	"alt/gateway/knowledge_reproject_gateway"
	"alt/gateway/knowledge_user_event_gateway"
	"alt/gateway/latest_article_gateway"
	"alt/gateway/morning_gateway"
	"alt/gateway/morning_letter_connect_gateway"
	"alt/gateway/opml_gateway"
	"alt/gateway/rag_connect_gateway"
	"alt/gateway/rag_gateway"
	"alt/gateway/rate_limiter_gateway"
	"alt/gateway/recall_candidate_gateway"
	"alt/gateway/recall_signal_gateway"
	"alt/gateway/recap_articles_gateway"
	"alt/gateway/recap_gateway"
	"alt/gateway/register_favorite_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/robots_txt_gateway"
	"alt/gateway/scraping_domain_gateway"
	"alt/gateway/scraping_policy_gateway"
	"alt/gateway/subscription_gateway"
	"alt/gateway/summary_version_gateway"
	"alt/gateway/tag_set_version_gateway"
	"alt/gateway/today_digest_gateway"
	"alt/gateway/trend_stats_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/gateway/user_feed_gateway"
	"alt/gateway/user_read_state_gateway"
	"alt/gateway/validate_fetch_rss_gateway"
	"alt/port/config_port"
	"alt/port/error_handler_port"
	"alt/port/event_publisher_port"
	"alt/port/morning_letter_port"
	"alt/port/rag_integration_port"
	"alt/port/rate_limiter_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/append_knowledge_event_usecase"
	"alt/usecase/knowledge_write_service_usecase"
	"alt/usecase/archive_article_usecase"
	"alt/usecase/archive_lens_usecase"
	"alt/usecase/cached_feed_list_usecase"
	"alt/usecase/create_lens_usecase"
	"alt/usecase/create_summary_version_usecase"
	"alt/usecase/create_tag_set_version_usecase"
	"alt/usecase/csrf_token_usecase"
	dashboard_usecase "alt/usecase/dashboard"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/fetch_article_summary_usecase"
	"alt/usecase/fetch_article_tags_usecase"
	"alt/usecase/fetch_article_usecase"
	"alt/usecase/fetch_articles_by_tag_usecase"
	"alt/usecase/fetch_articles_usecase"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_tags_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/fetch_inoreader_summary_usecase"
	"alt/usecase/fetch_latest_article_usecase"
	"alt/usecase/fetch_random_subscription_usecase"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/usecase/fetch_tag_cloud_usecase"
	"alt/usecase/fetch_trend_stats_usecase"
	"alt/usecase/get_knowledge_home_usecase"
	"alt/usecase/image_fetch_usecase"
	"alt/usecase/image_proxy_usecase"
	"alt/usecase/knowledge_audit_usecase"
	"alt/usecase/knowledge_backfill_usecase"
	"alt/usecase/knowledge_projection_health_usecase"
	"alt/usecase/knowledge_reproject_usecase"
	"alt/usecase/knowledge_slo_usecase"
	"alt/usecase/list_lenses_usecase"
	"alt/usecase/morning_usecase"
	"alt/usecase/opml_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/recall_dismiss_usecase"
	"alt/usecase/recall_rail_usecase"
	"alt/usecase/recall_snooze_usecase"
	"alt/usecase/recap_articles_usecase"
	"alt/usecase/recap_usecase"
	"alt/usecase/register_favorite_feed_usecase"
	"alt/usecase/register_feed_usecase"
	"alt/usecase/remove_favorite_feed_usecase"
	"alt/usecase/retrieve_context_usecase"
	"alt/usecase/scraping_domain_usecase"
	"alt/usecase/search_article_usecase"
	"alt/usecase/search_feed_usecase"
	"alt/usecase/select_lens_usecase"
	"alt/usecase/stream_article_tags_usecase"
	"alt/usecase/subscription_usecase"
	"alt/usecase/track_home_action_usecase"
	"alt/usecase/track_home_seen_usecase"
	"alt/usecase/update_lens_usecase"
	"alt/utils"
	"alt/utils/batch_article_fetcher"
	"alt/utils/image_proxy"
	altotel "alt/utils/otel"
	"alt/utils/rate_limiter"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationComponents struct {
	// Ports
	ConfigPort       config_port.ConfigPort
	RateLimiterPort  rate_limiter_port.RateLimiterPort
	ErrorHandlerPort error_handler_port.ErrorHandlerPort

	// Repository
	AltDBRepository *alt_db.AltDBRepository

	// Drivers
	KratosClient kratos_client.KratosClient

	// Ports
	RagIntegration   rag_integration_port.RagIntegrationPort
	RagConnectClient *rag_connect_gateway.Client
	StreamChatPort   morning_letter_port.StreamChatPort
	EventPublisher   event_publisher_port.EventPublisherPort

	// Usecases
	FetchSingleFeedUsecase              *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase               *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase         *fetch_feed_usecase.FetchFeedsListCursorUsecase
	FetchUnreadFeedsListCursorUsecase   *fetch_feed_usecase.FetchUnreadFeedsListCursorUsecase
	CachedFeedListUsecase               *cached_feed_list_usecase.CachedFeedListUsecase
	FetchReadFeedsListCursorUsecase     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsListCursorUsecase *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	RegisterFeedsUsecase                *register_feed_usecase.RegisterFeedsUsecase
	RegisterFavoriteFeedUsecase         *register_favorite_feed_usecase.RegisterFavoriteFeedUsecase
	RemoveFavoriteFeedUsecase           *remove_favorite_feed_usecase.RemoveFavoriteFeedUsecase
	ListFeedLinksUsecase                *feed_link_usecase.ListFeedLinksUsecase
	ListFeedLinksWithHealthUsecase      *feed_link_usecase.ListFeedLinksWithHealthUsecase
	DeleteFeedLinkUsecase               *feed_link_usecase.DeleteFeedLinkUsecase
	FeedsReadingStatusUsecase           *reading_status.FeedsReadingStatusUsecase
	ArticlesReadingStatusUsecase        *reading_status.ArticlesReadingStatusUsecase
	FeedsSummaryUsecase                 *fetch_feed_details_usecase.FeedsSummaryUsecase
	FeedAmountUsecase                   *fetch_feed_stats_usecase.FeedsCountUsecase
	UnsummarizedArticlesCountUsecase    *fetch_feed_stats_usecase.UnsummarizedArticlesCountUsecase
	SummarizedArticlesCountUsecase      *fetch_feed_stats_usecase.SummarizedArticlesCountUsecase
	TotalArticlesCountUsecase           *fetch_feed_stats_usecase.TotalArticlesCountUsecase
	TodayUnreadArticlesCountUsecase     *fetch_feed_stats_usecase.TodayUnreadArticlesCountUsecase
	TrendStatsUsecase                   *fetch_trend_stats_usecase.FetchTrendStatsUsecase
	FeedSearchUsecase                   *search_feed_usecase.SearchFeedMeilisearchUsecase
	ArticleSearchUsecase                *search_article_usecase.SearchArticleUsecase
	FetchFeedTagsUsecase                *fetch_feed_tags_usecase.FetchFeedTagsUsecase
	FetchInoreaderSummaryUsecase        fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	ImageFetchUsecase                   image_fetch_usecase.ImageFetchUsecaseInterface
	CSRFTokenUsecase                    *csrf_token_usecase.CSRFTokenUsecase
	ArticleUsecase                      fetch_article_usecase.ArticleUsecase
	ArchiveArticleUsecase               *archive_article_usecase.ArchiveArticleUsecase
	FetchArticlesCursorUsecase          *fetch_articles_usecase.FetchArticlesCursorUsecase
	FetchRecentArticlesUsecase          *fetch_recent_articles_usecase.FetchRecentArticlesUsecase
	RecapArticlesUsecase                *recap_articles_usecase.RecapArticlesUsecase
	RecapUsecase                        *recap_usecase.RecapUsecase
	MorningUsecase                      morning_letter_port.MorningUsecase
	ScrapingDomainUsecase               *scraping_domain_usecase.ScrapingDomainUsecase
	BatchArticleFetcher                 *batch_article_fetcher.BatchArticleFetcher
	RetrieveContextUsecase              retrieve_context_usecase.RetrieveContextUsecase
	AnswerChatUsecase                   answer_chat_usecase.AnswerChatUsecase
	FetchRandomSubscriptionUsecase      *fetch_random_subscription_usecase.FetchRandomSubscriptionUsecase
	FetchArticlesByTagUsecase           *fetch_articles_by_tag_usecase.FetchArticlesByTagUsecase
	FetchArticleTagsUsecase             *fetch_article_tags_usecase.FetchArticleTagsUsecase
	GetRecapJobsUsecase                 dashboard_usecase.GetRecapJobsUsecase
	ListSubscriptionsUsecase            *subscription_usecase.ListSubscriptionsUsecase
	SubscribeUsecase                    *subscription_usecase.SubscribeUsecase
	UnsubscribeUsecase                  *subscription_usecase.UnsubscribeUsecase

	DashboardMetricsUsecase    *dashboard_usecase.DashboardMetricsUsecase
	FetchLatestArticleUsecase  *fetch_latest_article_usecase.FetchLatestArticleUsecase
	FetchArticleSummaryUsecase *fetch_article_summary_usecase.FetchArticleSummaryUsecase
	StreamArticleTagsUsecase   *stream_article_tags_usecase.StreamArticleTagsUsecase
	FetchTagCloudUsecase       *fetch_tag_cloud_usecase.FetchTagCloudUsecase

	// OPML
	ExportOPMLUsecase *opml_usecase.ExportOPMLUsecase
	ImportOPMLUsecase *opml_usecase.ImportOPMLUsecase

	// Image Proxy
	ImageProxyUsecase *image_proxy_usecase.ImageProxyUsecase

	// Internal API gateway (service-to-service)
	InternalArticleGateway *internal_article_gateway.Gateway

	// Knowledge Home
	GetKnowledgeHomeUsecase           *get_knowledge_home_usecase.GetKnowledgeHomeUsecase
	TrackHomeSeenUsecase              *track_home_seen_usecase.TrackHomeSeenUsecase
	TrackHomeActionUsecase            *track_home_action_usecase.TrackHomeActionUsecase
	AppendKnowledgeEventUsecase       *append_knowledge_event_usecase.AppendKnowledgeEventUsecase
	CreateSummaryVersionUsecase       *create_summary_version_usecase.CreateSummaryVersionUsecase
	CreateTagSetVersionUsecase        *create_tag_set_version_usecase.CreateTagSetVersionUsecase
	KnowledgeEventGateway             *knowledge_event_gateway.Gateway
	KnowledgeProjectionGateway        *knowledge_projection_gateway.Gateway
	KnowledgeHomeGateway              *knowledge_home_gateway.Gateway
	TodayDigestGateway                *today_digest_gateway.Gateway
	FeatureFlagGateway                *feature_flag_gateway.Gateway
	KnowledgeBackfillGateway          *knowledge_backfill_gateway.Gateway
	KnowledgeProjectionVersionGateway *knowledge_projection_version_gateway.Gateway
	KnowledgeBackfillUsecase          *knowledge_backfill_usecase.Usecase
	KnowledgeProjectionHealthUsecase  *knowledge_projection_health_usecase.Usecase
	KnowledgeReprojectGateway         *knowledge_reproject_gateway.Gateway
	ReprojectUsecase                  *knowledge_reproject_usecase.Usecase
	SLOUsecase                        *knowledge_slo_usecase.Usecase
	AuditUsecase                      *knowledge_audit_usecase.Usecase

	// Phase 4: RecallRail, Lens, Stream, Supersede
	RecallRailUsecase      *recall_rail_usecase.RecallRailUsecase
	RecallSnoozeUsecase    *recall_snooze_usecase.RecallSnoozeUsecase
	RecallDismissUsecase   *recall_dismiss_usecase.RecallDismissUsecase
	CreateLensUsecase      *create_lens_usecase.CreateLensUsecase
	UpdateLensUsecase      *update_lens_usecase.UpdateLensUsecase
	ListLensesUsecase      *list_lenses_usecase.ListLensesUsecase
	SelectLensUsecase      *select_lens_usecase.SelectLensUsecase
	ArchiveLensUsecase     *archive_lens_usecase.ArchiveLensUsecase
	RecallSignalGateway    *recall_signal_gateway.Gateway
	RecallCandidateGateway *recall_candidate_gateway.Gateway
	SummaryVersionGateway  *summary_version_gateway.Gateway
	TagSetVersionGateway   *tag_set_version_gateway.Gateway
	KnowledgeLensGateway   *knowledge_lens_gateway.Gateway

	// Knowledge Sovereign
	KnowledgeSovereignGateway    *knowledge_sovereign_gateway.Gateway
	KnowledgeWriteServiceUsecase *knowledge_write_service_usecase.KnowledgeWriteServiceUsecase

	// Observability
	KnowledgeHomeMetrics *altotel.KnowledgeHomeMetrics
}

func NewApplicationComponents(pool *pgxpool.Pool) *ApplicationComponents {
	altDBRepository := alt_db.NewAltDBRepository(pool)
	httpClient := utils.NewHTTPClientFactory().CreateHTTPClient()

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
	rateLimiter := rate_limiter.NewHostRateLimiter(rateLimitConfig.ExternalAPIInterval, rateLimitConfig.ExternalAPIBurst)
	rateLimiterPort := rate_limiter_gateway.NewRateLimiterGateway(rateLimiter)

	// Create the concrete gateway implementations with rate limiting
	feedFetcherGatewayImpl := fetch_feed_gateway.NewSingleFeedGatewayWithRateLimiter(pool, rateLimiter)
	fetchFeedsListGatewayImpl := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(pool, rateLimiter)
	feedPageCacheGatewayImpl := feed_page_cache_gateway.NewGateway(altDBRepository)
	userReadStateGatewayImpl := user_read_state_gateway.NewGateway(altDBRepository)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)
	fetchFeedsListUsecase := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGatewayImpl)
	fetchFeedsListCursorUsecase := fetch_feed_usecase.NewFetchFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	fetchUnreadFeedsListCursorUsecase := fetch_feed_usecase.NewFetchUnreadFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	cachedFeedListUsecase := cached_feed_list_usecase.NewCachedFeedListUsecase(feedPageCacheGatewayImpl, userReadStateGatewayImpl, fetchFeedsListGatewayImpl)
	fetchReadFeedsListCursorUsecase := fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	fetchFavoriteFeedsListCursorUsecase := fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(fetchFeedsListGatewayImpl)

	validateAndFetchRSSGatewayImpl := validate_fetch_rss_gateway.NewValidateAndFetchRSSGateway()
	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(pool)
	registerFavoriteFeedGatewayImpl := register_favorite_feed_gateway.NewRegisterFavoriteFeedGateway(pool)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(validateAndFetchRSSGatewayImpl, registerFeedLinkGatewayImpl, registerFeedsGatewayImpl)
	registerFeedsUsecase.SetFeedLinkIDResolver(altDBRepository)
	registerFeedsUsecase.SetFeedLinkAvailabilityPort(altDBRepository)
	registerFeedsUsecase.SetFeedPageInvalidator(feedPageCacheGatewayImpl)
	registerFavoriteFeedUsecase := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(registerFavoriteFeedGatewayImpl)
	removeFavoriteFeedUsecase := remove_favorite_feed_usecase.NewRemoveFavoriteFeedUsecase(registerFavoriteFeedGatewayImpl)
	feedLinkGatewayImpl := feed_link_gateway.NewFeedLinkGateway(pool)
	listFeedLinksUsecase := feed_link_usecase.NewListFeedLinksUsecase(feedLinkGatewayImpl)
	listFeedLinksWithHealthUsecase := feed_link_usecase.NewListFeedLinksWithHealthUsecase(feedLinkGatewayImpl)

	updateFeedStatusGatewayImpl := update_feed_status_gateway.NewUpdateFeedStatusGateway(pool)
	feedsReadingStatusUsecase := reading_status.NewFeedsReadingStatusUsecase(updateFeedStatusGatewayImpl)

	// Article reading status usecase (uses AltDBRepository which implements UpdateArticleStatusPort)
	articlesReadingStatusUsecase := reading_status.NewArticlesReadingStatusUsecase(altDBRepository)

	feedSummaryGatewayImpl := fetch_feed_detail_gateway.NewFeedSummaryGateway(pool)
	feedsSummaryUsecase := fetch_feed_details_usecase.NewFeedsSummaryUsecase(feedSummaryGatewayImpl)

	feedAmountGatewayImpl := feed_stats_gateway.NewFeedAmountGateway(pool)
	feedsCountUsecase := fetch_feed_stats_usecase.NewFeedsCountUsecase(feedAmountGatewayImpl)

	unsummarizedArticlesCountGatewayImpl := feed_stats_gateway.NewUnsummarizedArticlesCountGateway(pool)
	unsummarizedArticlesCountUsecase := fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(unsummarizedArticlesCountGatewayImpl)

	summarizedArticlesCountGatewayImpl := feed_stats_gateway.NewSummarizedArticlesCountGateway(pool)
	summarizedArticlesCountUsecase := fetch_feed_stats_usecase.NewSummarizedArticlesCountUsecase(summarizedArticlesCountGatewayImpl)

	totalArticlesCountGatewayImpl := feed_stats_gateway.NewTotalArticlesCountGateway(pool)
	totalArticlesCountUsecase := fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(totalArticlesCountGatewayImpl)

	todayUnreadArticlesCountGatewayImpl := feed_stats_gateway.NewTodayUnreadArticlesCountGateway(pool)
	todayUnreadArticlesCountUsecase := fetch_feed_stats_usecase.NewTodayUnreadArticlesCountUsecase(todayUnreadArticlesCountGatewayImpl)

	// Trend stats components
	trendStatsGatewayImpl := trend_stats_gateway.NewTrendStatsGateway(pool)
	trendStatsUsecase := fetch_trend_stats_usecase.NewFetchTrendStatsUsecase(trendStatsGatewayImpl)

	// Article search (Meilisearch-based via search-indexer)
	searchIndexerDriver := search_indexer_connect.NewConnectSearchIndexerDriver(cfg.SearchIndexer.ConnectURL)
	articleSearchUsecase := search_article_usecase.NewSearchArticleUsecase(searchIndexerDriver)

	// Feed search (Meilisearch-based via search-indexer)
	searchFeedMeilisearchGatewayImpl := feed_search_gateway.NewSearchFeedMeilisearchGateway(searchIndexerDriver)
	feedURLLinkGatewayImpl := feed_url_link_gateway.NewFeedURLLinkGateway(altDBRepository)
	feedSearchUsecase := search_feed_usecase.NewSearchFeedMeilisearchUsecase(searchFeedMeilisearchGatewayImpl, feedURLLinkGatewayImpl)

	feedURLToIDGatewayImpl := feed_url_to_id_gateway.NewFeedURLToIDGateway(altDBRepository)
	fetchFeedTagsGatewayImpl := fetch_feed_tags_gateway.NewFetchFeedTagsGateway(altDBRepository)
	fetchFeedTagsUsecase := fetch_feed_tags_usecase.NewFetchFeedTagsUsecase(feedURLToIDGatewayImpl, fetchFeedTagsGatewayImpl)

	// Robots.txt gateway (used by multiple components)
	robotsTxtGatewayImpl := robots_txt_gateway.NewRobotsTxtGateway(httpClient)

	// RAG Integration
	ragClient, err := rag_gateway.NewClientWithResponses(cfg.Rag.OrchestratorURL)
	if err != nil {
		// Log error but proceed (fail-open or panic depending on strictness - here we panic as it is config error likely)
		panic("Failed to create RAG client: " + err.Error())
	}
	ragAdapterImpl := augur_adapter.NewAugurAdapter(ragClient)

	ragRetrieveContextUsecase := retrieve_context_usecase.NewRetrieveContextUsecase(searchFeedMeilisearchGatewayImpl, ragAdapterImpl)
	answerChatUsecase := answer_chat_usecase.NewAnswerChatUsecase(ragAdapterImpl)

	// RAG Connect-RPC client (for direct Connect-RPC communication with rag-orchestrator)
	ragConnectClient := rag_connect_gateway.NewClient(cfg.Rag.OrchestratorConnectURL, slog.Default())

	fetchArticleGatewayImpl := fetch_article_gateway.NewFetchArticleGateway(rateLimiter, httpClient)
	// fetchArticleUsecase is initialized below after scrapingDomainGatewayImpl is created
	var fetchArticleUsecase fetch_article_usecase.ArticleUsecase
	archiveArticleGatewayImpl := archive_article_gateway.NewArchiveArticleGateway(altDBRepository)
	archiveArticleUsecase := archive_article_usecase.NewArchiveArticleUsecase(fetchArticleGatewayImpl, archiveArticleGatewayImpl)

	// Batch article fetcher for efficient multi-URL fetching with domain-based rate limiting
	batchArticleFetcher := batch_article_fetcher.NewBatchArticleFetcher(rateLimiter, httpClient)

	// Fetch articles with cursor components
	fetchArticlesGatewayImpl := article_gateway.NewFetchArticlesGateway(pool)
	articleContentCacheGatewayImpl := article_content_cache_gateway.NewGateway(altDBRepository)
	fetchArticlesCursorUsecase := fetch_articles_usecase.NewFetchArticlesCursorUsecaseWithCache(fetchArticlesGatewayImpl, articleContentCacheGatewayImpl)

	// Fetch recent articles components (for rag-orchestrator temporal topics)
	fetchRecentArticlesGatewayImpl := fetch_recent_articles_gateway.NewFetchRecentArticlesGateway(pool)
	fetchRecentArticlesUsecase := fetch_recent_articles_usecase.NewFetchRecentArticlesUsecase(fetchRecentArticlesGatewayImpl)
	recapArticlesGateway := recap_articles_gateway.NewGateway(altDBRepository)
	recapUsecaseCfg := recap_articles_usecase.Config{
		DefaultPageSize: cfg.Recap.DefaultPageSize,
		MaxPageSize:     cfg.Recap.MaxPageSize,
		MaxRangeDays:    cfg.Recap.MaxRangeDays,
	}
	recapArticlesUsecase := recap_articles_usecase.NewRecapArticlesUsecase(recapArticlesGateway, recapUsecaseCfg)

	// Recap 7-day summary components
	recapGateway := recap_gateway.NewRecapGateway(searchIndexerDriver)
	recapUsecase := recap_usecase.NewRecapUsecase(recapGateway)

	// Fetch inoreader summary components
	fetchInoreaderSummaryGatewayImpl := fetch_inoreader_summary_gateway.NewInoreaderSummaryGateway(altDBRepository)
	fetchInoreaderSummaryUsecase := fetch_inoreader_summary_usecase.NewFetchInoreaderSummaryUsecase(fetchInoreaderSummaryGatewayImpl)

	// CSRF token components
	csrfTokenDriver := csrf_token_driver.NewInMemoryCSRFTokenDriver()
	csrfTokenGateway := csrf_token_gateway.NewCSRFTokenGateway(csrfTokenDriver)
	csrfTokenUsecase := csrf_token_usecase.NewCSRFTokenUsecase(csrfTokenGateway)

	// Image fetch components
	imageHTTPClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	imageFetchGateway := image_fetch_gateway.NewImageFetchGateway(imageHTTPClient)
	imageFetchUsecase := image_fetch_usecase.NewImageFetchUsecase(imageFetchGateway)

	// Image proxy components
	// CDN public images are fetched on-demand per user action, not crawled.
	// 1 req/s/host is conservative enough and avoids context deadline exceeded
	// when multiple images from the same host are requested concurrently.
	imageProxyRateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Second)
	var imageProxyUsecaseInstance *image_proxy_usecase.ImageProxyUsecase
	if cfg.ImageProxy.Enabled && cfg.ImageProxy.Secret != "" {
		imageProxySigner := image_proxy.NewSigner(cfg.ImageProxy.Secret)
		imageProxyCacheGateway := image_proxy_gateway.NewCacheGateway(altDBRepository)
		imageProxyProcessingGateway := image_proxy_gateway.NewProcessingGateway()
		imageProxyDynamicDomainGateway := image_proxy_gateway.NewDynamicDomainGateway(altDBRepository)
		imageProxyUsecaseInstance = image_proxy_usecase.NewImageProxyUsecase(
			imageFetchGateway,
			imageProxyProcessingGateway,
			imageProxyCacheGateway,
			imageProxySigner,
			imageProxyDynamicDomainGateway,
			imageProxyRateLimiter,
			cfg.ImageProxy.MaxWidth,
			cfg.ImageProxy.WebPQuality,
			cfg.ImageProxy.CacheTTLMin,
		)
	}

	// Morning letter components
	userFeedGatewayImpl := user_feed_gateway.NewGateway(altDBRepository)
	morningGatewayImpl := morning_gateway.NewMorningGateway(pool)
	morningUsecase := morning_usecase.NewMorningUsecase(morningGatewayImpl, userFeedGatewayImpl)

	// Scraping domain components
	scrapingDomainGatewayImpl := scraping_domain_gateway.NewScrapingDomainGateway(altDBRepository)
	feedLinkDomainGatewayImpl := feed_link_domain_gateway.NewFeedLinkDomainGateway(altDBRepository)
	scrapingDomainUsecase := scraping_domain_usecase.NewScrapingDomainUsecaseWithFeedLinkDomain(scrapingDomainGatewayImpl, robotsTxtGatewayImpl, feedLinkDomainGatewayImpl)

	// Wire ScrapingPolicyGateway into ArticleUsecase (uses cached robots.txt from scraping_domains)
	scrapingPolicyGatewayImpl := scraping_policy_gateway.NewScrapingPolicyGateway(scrapingDomainGatewayImpl)
	fetchArticleUsecase = fetch_article_usecase.NewArticleUsecaseWithScrapingPolicy(fetchArticleGatewayImpl, robotsTxtGatewayImpl, altDBRepository, ragAdapterImpl, scrapingPolicyGatewayImpl)

	// MorningLetter Connect-RPC gateway (calls rag-orchestrator)
	morningLetterConnectGateway := morning_letter_connect_gateway.NewGateway(cfg.Rag.OrchestratorConnectURL, slog.Default())

	// MQ-Hub event publisher (optional, fail-open if disabled)
	mqhubClient := mqhub_connect.NewClient(cfg.MQHub.ConnectURL, cfg.MQHub.Enabled)
	eventPublisherGatewayImpl := event_publisher_gateway.NewEventPublisherGateway(mqhubClient, slog.Default())

	// Auth-hub client for identity management (abstracts Kratos)
	kratosClientImpl := kratos_client.NewKratosClient(cfg.AuthHub.URL, cfg.Auth.BackendTokenSecret)

	// Random subscription components (for Tag Trail feature)
	fetchRandomSubscriptionGatewayImpl := fetch_random_subscription_gateway.NewFetchRandomSubscriptionGateway(altDBRepository)
	fetchRandomSubscriptionUsecase := fetch_random_subscription_usecase.NewFetchRandomSubscriptionUsecase(fetchRandomSubscriptionGatewayImpl)

	// Articles by tag components (for Tag Trail feature)
	fetchArticlesByTagGatewayImpl := fetch_articles_by_tag_gateway.NewFetchArticlesByTagGateway(altDBRepository)
	fetchArticlesByTagUsecase := fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(fetchArticlesByTagGatewayImpl)

	// Tag cloud components (for Tag Verse feature)
	fetchTagCloudGatewayImpl := fetch_tag_cloud_gateway.NewFetchTagCloudGateway(altDBRepository)
	fetchTagCloudUsecase := fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(fetchTagCloudGatewayImpl, 30*time.Minute)

	// Article tags components (for Tag Trail feature)
	// Use gateway with mq-hub client to enable on-the-fly tag generation (ADR-168)
	fetchArticleTagsConfig := fetch_article_tags_gateway.DefaultConfig()
	fetchArticleTagsGatewayImpl := fetch_article_tags_gateway.NewFetchArticleTagsGatewayWithMQHub(
		altDBRepository,
		mqhubClient,
		fetchArticleTagsConfig,
	)
	fetchArticleTagsUsecase := fetch_article_tags_usecase.NewFetchArticleTagsUsecase(fetchArticleTagsGatewayImpl)

	// Dashboard recap jobs components
	recapJobDriver := recap_job_driver.NewRecapJobGateway(cfg.Recap.WorkerURL)
	getRecapJobsUsecase := dashboard_usecase.NewGetRecapJobsUsecase(recapJobDriver)

	// Internal article API gateway (for BackendInternalService)
	internalArticleGatewayImpl := internal_article_gateway.NewGateway(altDBRepository)

	// Dashboard metrics components
	dashboardGatewayImpl := dashboard_gateway.NewDashboardGateway()
	dashboardMetricsUsecase := dashboard_usecase.NewDashboardMetricsUsecase(dashboardGatewayImpl)

	// Article summary components (for FetchArticleSummary)
	articleSummaryGatewayImpl := article_summary_gateway.NewGateway(altDBRepository)
	fetchArticleSummaryUsecase := fetch_article_summary_usecase.NewFetchArticleSummaryUsecase(articleSummaryGatewayImpl)

	// Latest article components (for FetchRandomFeed)
	latestArticleGatewayImpl := latest_article_gateway.NewGateway(altDBRepository)
	fetchLatestArticleUsecase := fetch_latest_article_usecase.NewFetchLatestArticleUsecase(latestArticleGatewayImpl)

	// Stream article tags components (cached check + on-the-fly generation)
	cachedArticleTagsGatewayImpl := cached_article_tags_gateway.NewGateway(altDBRepository)
	streamArticleTagsUsecase := stream_article_tags_usecase.NewStreamArticleTagsUsecase(
		cachedArticleTagsGatewayImpl,
		fetchArticleTagsGatewayImpl,
	)

	// Subscription components
	subscriptionGatewayImpl := subscription_gateway.NewSubscriptionGateway(pool)
	listSubscriptionsUsecase := subscription_usecase.NewListSubscriptionsUsecase(subscriptionGatewayImpl)
	subscribeUsecase := subscription_usecase.NewSubscribeUsecase(subscriptionGatewayImpl)
	unsubscribeUsecase := subscription_usecase.NewUnsubscribeUsecase(subscriptionGatewayImpl)
	deleteFeedLinkUsecase := feed_link_usecase.NewDeleteFeedLinkUsecase(subscriptionGatewayImpl)

	// OPML components
	opmlExportGateway := opml_gateway.NewExportGateway(pool)
	opmlImportGateway := opml_gateway.NewImportGateway(pool)
	exportOPMLUsecase := opml_usecase.NewExportOPMLUsecase(opmlExportGateway)
	importOPMLUsecase := opml_usecase.NewImportOPMLUsecase(opmlImportGateway)

	// Knowledge Home components
	knowledgeEventGw := knowledge_event_gateway.NewGateway(altDBRepository)
	knowledgeHomeGw := knowledge_home_gateway.NewGateway(altDBRepository)
	todayDigestGw := today_digest_gateway.NewGateway(altDBRepository)
	knowledgeUserEventGw := knowledge_user_event_gateway.NewGateway(altDBRepository)
	summaryVersionGw := summary_version_gateway.NewGateway(altDBRepository)
	tagSetVersionGw := tag_set_version_gateway.NewGateway(altDBRepository)
	knowledgeProjectionGw := knowledge_projection_gateway.NewGateway(altDBRepository)
	featureFlagGw := feature_flag_gateway.NewGateway(&cfg.KnowledgeHome)
	knowledgeBackfillGw := knowledge_backfill_gateway.NewGateway(altDBRepository)
	knowledgeProjectionVersionGw := knowledge_projection_version_gateway.NewGateway(altDBRepository)
	knowledgeLensGw := knowledge_lens_gateway.NewGateway(altDBRepository)

	getKnowledgeHomeUsecase := get_knowledge_home_usecase.NewGetKnowledgeHomeUsecase(knowledgeHomeGw, todayDigestGw, knowledgeLensGw, todayDigestGw, todayDigestGw)
	trackHomeSeenUsecase := track_home_seen_usecase.NewTrackHomeSeenUsecase(knowledgeUserEventGw, featureFlagGw)
	trackHomeActionUsecase := track_home_action_usecase.NewTrackHomeActionUsecase(knowledgeUserEventGw, knowledgeEventGw, featureFlagGw)
	appendKnowledgeEventUsecase := append_knowledge_event_usecase.NewAppendKnowledgeEventUsecase(knowledgeEventGw)
	createSummaryVersionUsecase := create_summary_version_usecase.NewCreateSummaryVersionUsecase(summaryVersionGw, knowledgeEventGw, summaryVersionGw)
	createTagSetVersionUsecase := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetVersionGw, knowledgeEventGw, tagSetVersionGw)
	knowledgeBackfillUsecase := knowledge_backfill_usecase.NewUsecase(
		knowledgeBackfillGw,
		knowledgeBackfillGw,
		knowledgeBackfillGw,
		knowledgeBackfillGw,
		knowledgeBackfillGw,
		knowledgeEventGw,
	)
	knowledgeProjectionHealthUsecase := knowledge_projection_health_usecase.NewUsecase(knowledgeProjectionVersionGw, knowledgeProjectionGw, knowledgeBackfillGw)

	// Phase 5: Reproject, SLO, Audit components
	knowledgeReprojectGw := knowledge_reproject_gateway.NewGateway(altDBRepository)
	reprojectUsecase := knowledge_reproject_usecase.NewUsecase(
		knowledgeReprojectGw,
		knowledgeReprojectGw,
		knowledgeReprojectGw,
		knowledgeReprojectGw,
		knowledgeReprojectGw,
		knowledgeProjectionVersionGw,
		knowledgeProjectionVersionGw,
	)
	sloUsecase := knowledge_slo_usecase.NewUsecase(altDBRepository)
	auditUsecase := knowledge_audit_usecase.NewUsecase(knowledgeReprojectGw, knowledgeReprojectGw)

	// Phase 4: RecallRail, Lens, Stream, Supersede components
	recallSignalGw := recall_signal_gateway.NewGateway(altDBRepository)
	recallCandidateGw := recall_candidate_gateway.NewGateway(altDBRepository)

	recallRailUsecase := recall_rail_usecase.NewRecallRailUsecase(recallCandidateGw, featureFlagGw)
	recallSnoozeUsecase := recall_snooze_usecase.NewRecallSnoozeUsecase(recallCandidateGw, knowledgeEventGw)
	recallDismissUsecase := recall_dismiss_usecase.NewRecallDismissUsecase(recallCandidateGw, knowledgeEventGw)
	createLensUsecase := create_lens_usecase.NewCreateLensUsecase(knowledgeLensGw, knowledgeLensGw)
	updateLensUsecase := update_lens_usecase.NewUpdateLensUsecase(knowledgeLensGw, knowledgeLensGw)
	listLensesUsecase := list_lenses_usecase.NewListLensesUsecase(knowledgeLensGw, knowledgeLensGw)
	selectLensUsecase := select_lens_usecase.NewSelectLensUsecase(knowledgeLensGw, knowledgeLensGw, knowledgeLensGw, knowledgeLensGw)
	archiveLensUsecase := archive_lens_usecase.NewArchiveLensUsecase(knowledgeLensGw, knowledgeLensGw)

	// Knowledge Home metrics (optional, fail-open)
	var knowledgeHomeMetrics *altotel.KnowledgeHomeMetrics
	if m, err := altotel.NewKnowledgeHomeMetrics(); err != nil {
		slog.Warn("failed to initialize KnowledgeHomeMetrics, continuing without metrics", "error", err)
	} else {
		knowledgeHomeMetrics = m
	}

	// Wire recall signal port: TrackHomeAction appends recall signals (fire-and-forget)
	trackHomeActionUsecase.SetRecallSignalPort(recallSignalGw)
	trackHomeActionUsecase.SetDismissPort(knowledgeHomeGw, knowledgeProjectionVersionGw)

	// Knowledge Sovereign wiring
	knowledgeSovereignGw := knowledge_sovereign_gateway.NewGateway(altDBRepository)
	knowledgeWriteServiceUsecase := knowledge_write_service_usecase.NewKnowledgeWriteServiceUsecase(
		knowledgeSovereignGw, knowledgeSovereignGw, knowledgeSovereignGw,
		knowledgeSovereignGw, knowledgeSovereignGw,
	)
	trackHomeActionUsecase.SetCurationMutator(knowledgeSovereignGw)

	// Wire auto-subscribe: Usecase delegates subscription to SubscriptionPort
	registerFeedsUsecase.SetSubscriptionPort(subscriptionGatewayImpl)
	// Wire event publisher: Usecase publishes ArticleCreated events (fire-and-forget)
	registerFeedsUsecase.SetEventPublisher(eventPublisherGatewayImpl)

	return &ApplicationComponents{
		// Ports
		ConfigPort:       configPort,
		RateLimiterPort:  rateLimiterPort,
		ErrorHandlerPort: errorHandlerPort,

		// Repository
		AltDBRepository: altDBRepository,

		// Drivers
		KratosClient: kratosClientImpl,

		// Ports
		RagIntegration:   ragAdapterImpl,
		RagConnectClient: ragConnectClient,
		StreamChatPort:   morningLetterConnectGateway,
		EventPublisher:   eventPublisherGatewayImpl,

		// Usecases
		FetchSingleFeedUsecase:              fetchSingleFeedUsecase,
		FetchFeedsListUsecase:               fetchFeedsListUsecase,
		FetchFeedsListCursorUsecase:         fetchFeedsListCursorUsecase,
		FetchUnreadFeedsListCursorUsecase:   fetchUnreadFeedsListCursorUsecase,
		CachedFeedListUsecase:               cachedFeedListUsecase,
		FetchReadFeedsListCursorUsecase:     fetchReadFeedsListCursorUsecase,
		FetchFavoriteFeedsListCursorUsecase: fetchFavoriteFeedsListCursorUsecase,
		RegisterFeedsUsecase:                registerFeedsUsecase,
		RegisterFavoriteFeedUsecase:         registerFavoriteFeedUsecase,
		RemoveFavoriteFeedUsecase:           removeFavoriteFeedUsecase,
		ListFeedLinksUsecase:                listFeedLinksUsecase,
		ListFeedLinksWithHealthUsecase:      listFeedLinksWithHealthUsecase,
		DeleteFeedLinkUsecase:               deleteFeedLinkUsecase,
		FeedsReadingStatusUsecase:           feedsReadingStatusUsecase,
		ArticlesReadingStatusUsecase:        articlesReadingStatusUsecase,
		FeedsSummaryUsecase:                 feedsSummaryUsecase,
		FeedAmountUsecase:                   feedsCountUsecase,
		UnsummarizedArticlesCountUsecase:    unsummarizedArticlesCountUsecase,
		SummarizedArticlesCountUsecase:      summarizedArticlesCountUsecase,
		TotalArticlesCountUsecase:           totalArticlesCountUsecase,
		TodayUnreadArticlesCountUsecase:     todayUnreadArticlesCountUsecase,
		TrendStatsUsecase:                   trendStatsUsecase,
		FeedSearchUsecase:                   feedSearchUsecase,
		ArticleSearchUsecase:                articleSearchUsecase,
		FetchFeedTagsUsecase:                fetchFeedTagsUsecase,
		FetchInoreaderSummaryUsecase:        fetchInoreaderSummaryUsecase,
		ImageFetchUsecase:                   imageFetchUsecase,
		CSRFTokenUsecase:                    csrfTokenUsecase,
		ArticleUsecase:                      fetchArticleUsecase,
		ArchiveArticleUsecase:               archiveArticleUsecase,
		FetchArticlesCursorUsecase:          fetchArticlesCursorUsecase,
		FetchRecentArticlesUsecase:          fetchRecentArticlesUsecase,
		RecapArticlesUsecase:                recapArticlesUsecase,
		RecapUsecase:                        recapUsecase,
		MorningUsecase:                      morningUsecase,
		ScrapingDomainUsecase:               scrapingDomainUsecase,
		BatchArticleFetcher:                 batchArticleFetcher,
		RetrieveContextUsecase:              ragRetrieveContextUsecase,
		AnswerChatUsecase:                   answerChatUsecase,
		FetchRandomSubscriptionUsecase:      fetchRandomSubscriptionUsecase,
		FetchArticlesByTagUsecase:           fetchArticlesByTagUsecase,
		FetchArticleTagsUsecase:             fetchArticleTagsUsecase,
		GetRecapJobsUsecase:                 getRecapJobsUsecase,
		ListSubscriptionsUsecase:            listSubscriptionsUsecase,
		SubscribeUsecase:                    subscribeUsecase,
		UnsubscribeUsecase:                  unsubscribeUsecase,

		DashboardMetricsUsecase:    dashboardMetricsUsecase,
		FetchLatestArticleUsecase:  fetchLatestArticleUsecase,
		FetchArticleSummaryUsecase: fetchArticleSummaryUsecase,
		StreamArticleTagsUsecase:   streamArticleTagsUsecase,
		FetchTagCloudUsecase:       fetchTagCloudUsecase,

		// OPML
		ExportOPMLUsecase: exportOPMLUsecase,
		ImportOPMLUsecase: importOPMLUsecase,

		// Image Proxy
		ImageProxyUsecase: imageProxyUsecaseInstance,

		// Internal API gateway
		InternalArticleGateway: internalArticleGatewayImpl,

		// Knowledge Home
		GetKnowledgeHomeUsecase:           getKnowledgeHomeUsecase,
		TrackHomeSeenUsecase:              trackHomeSeenUsecase,
		TrackHomeActionUsecase:            trackHomeActionUsecase,
		AppendKnowledgeEventUsecase:       appendKnowledgeEventUsecase,
		CreateSummaryVersionUsecase:       createSummaryVersionUsecase,
		CreateTagSetVersionUsecase:        createTagSetVersionUsecase,
		KnowledgeEventGateway:             knowledgeEventGw,
		KnowledgeProjectionGateway:        knowledgeProjectionGw,
		KnowledgeHomeGateway:              knowledgeHomeGw,
		TodayDigestGateway:                todayDigestGw,
		FeatureFlagGateway:                featureFlagGw,
		KnowledgeBackfillGateway:          knowledgeBackfillGw,
		KnowledgeProjectionVersionGateway: knowledgeProjectionVersionGw,
		KnowledgeBackfillUsecase:          knowledgeBackfillUsecase,
		KnowledgeProjectionHealthUsecase:  knowledgeProjectionHealthUsecase,
		KnowledgeReprojectGateway:         knowledgeReprojectGw,
		ReprojectUsecase:                  reprojectUsecase,
		SLOUsecase:                        sloUsecase,
		AuditUsecase:                      auditUsecase,

		// Phase 4
		RecallRailUsecase:      recallRailUsecase,
		RecallSnoozeUsecase:    recallSnoozeUsecase,
		RecallDismissUsecase:   recallDismissUsecase,
		CreateLensUsecase:      createLensUsecase,
		UpdateLensUsecase:      updateLensUsecase,
		ListLensesUsecase:      listLensesUsecase,
		SelectLensUsecase:      selectLensUsecase,
		ArchiveLensUsecase:     archiveLensUsecase,
		RecallSignalGateway:    recallSignalGw,
		RecallCandidateGateway: recallCandidateGw,
		SummaryVersionGateway:  summaryVersionGw,
		TagSetVersionGateway:   tagSetVersionGw,
		KnowledgeLensGateway:   knowledgeLensGw,

		// Knowledge Sovereign
		KnowledgeSovereignGateway:    knowledgeSovereignGw,
		KnowledgeWriteServiceUsecase: knowledgeWriteServiceUsecase,

		// Observability
		KnowledgeHomeMetrics: knowledgeHomeMetrics,
	}
}
