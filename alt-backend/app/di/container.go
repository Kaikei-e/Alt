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
	"alt/gateway/article_gateway"
	"alt/gateway/config_gateway"
	"alt/gateway/csrf_token_gateway"
	"alt/gateway/error_handler_gateway"
	"alt/gateway/event_publisher_gateway"
	"alt/gateway/feed_link_domain_gateway"
	"alt/gateway/feed_link_gateway"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_link_gateway"
	"alt/gateway/feed_url_to_id_gateway"
	"alt/gateway/fetch_article_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/fetch_feed_tags_gateway"
	"alt/gateway/fetch_inoreader_summary_gateway"
	"alt/gateway/fetch_recent_articles_gateway"
	"alt/gateway/image_fetch_gateway"
	"alt/gateway/image_proxy_gateway"
	"alt/gateway/morning_gateway"
	"alt/gateway/morning_letter_connect_gateway"
	"alt/gateway/rag_connect_gateway"
	"alt/gateway/rag_gateway"
	"alt/gateway/rate_limiter_gateway"
	"alt/gateway/recap_articles_gateway"
	"alt/gateway/recap_gateway"
	"alt/gateway/register_favorite_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/robots_txt_gateway"
	"alt/gateway/scraping_domain_gateway"
	"alt/gateway/scraping_policy_gateway"
	"alt/gateway/trend_stats_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/gateway/user_feed_gateway"
	"alt/port/config_port"
	"alt/port/error_handler_port"
	"alt/port/event_publisher_port"
	"alt/port/morning_letter_port"
	"alt/port/rag_integration_port"
	"alt/port/rate_limiter_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/archive_article_usecase"
	"alt/usecase/csrf_token_usecase"
	dashboard_usecase "alt/usecase/dashboard"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/fetch_article_usecase"
	"alt/usecase/fetch_articles_usecase"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_tags_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/fetch_inoreader_summary_usecase"
	"alt/usecase/fetch_random_subscription_usecase"
	"alt/usecase/fetch_articles_by_tag_usecase"
	"alt/usecase/fetch_article_tags_usecase"
	"alt/gateway/fetch_random_subscription_gateway"
	"alt/gateway/fetch_articles_by_tag_gateway"
	"alt/gateway/article_summary_gateway"
	"alt/gateway/cached_article_tags_gateway"
	"alt/gateway/dashboard_gateway"
	"alt/gateway/fetch_article_tags_gateway"
	"alt/gateway/internal_article_gateway"
	"alt/gateway/latest_article_gateway"
	"alt/gateway/subscription_gateway"
	"alt/usecase/fetch_article_summary_usecase"
	"alt/usecase/fetch_latest_article_usecase"
	"alt/usecase/stream_article_tags_usecase"
	"alt/usecase/fetch_recent_articles_usecase"
	"alt/usecase/fetch_trend_stats_usecase"
	"alt/usecase/image_fetch_usecase"
	"alt/usecase/image_proxy_usecase"
	"alt/usecase/morning_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/recap_articles_usecase"
	"alt/usecase/recap_usecase"
	"alt/usecase/register_favorite_feed_usecase"
	"alt/usecase/register_feed_usecase"
	"alt/usecase/retrieve_context_usecase"
	"alt/usecase/scraping_domain_usecase"
	"alt/usecase/search_article_usecase"
	"alt/usecase/search_feed_usecase"
	"alt/usecase/subscription_usecase"
	"alt/utils"
	"alt/utils/batch_article_fetcher"
	"alt/utils/image_proxy"
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
	RagIntegration rag_integration_port.RagIntegrationPort
	RagConnectClient *rag_connect_gateway.Client
	StreamChatPort   morning_letter_port.StreamChatPort
	EventPublisher   event_publisher_port.EventPublisherPort

	// Usecases
	FetchSingleFeedUsecase              *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase               *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase         *fetch_feed_usecase.FetchFeedsListCursorUsecase
	FetchUnreadFeedsListCursorUsecase   *fetch_feed_usecase.FetchUnreadFeedsListCursorUsecase
	FetchReadFeedsListCursorUsecase     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsListCursorUsecase *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	RegisterFeedsUsecase                *register_feed_usecase.RegisterFeedsUsecase
	RegisterFavoriteFeedUsecase         *register_favorite_feed_usecase.RegisterFavoriteFeedUsecase
	ListFeedLinksUsecase                *feed_link_usecase.ListFeedLinksUsecase
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

	DashboardMetricsUsecase             *dashboard_usecase.DashboardMetricsUsecase
	FetchLatestArticleUsecase           *fetch_latest_article_usecase.FetchLatestArticleUsecase
	FetchArticleSummaryUsecase          *fetch_article_summary_usecase.FetchArticleSummaryUsecase
	StreamArticleTagsUsecase            *stream_article_tags_usecase.StreamArticleTagsUsecase

	// Image Proxy
	ImageProxyUsecase                   *image_proxy_usecase.ImageProxyUsecase

	// Internal API gateway (service-to-service)
	InternalArticleGateway              *internal_article_gateway.Gateway
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
	rateLimiter := rate_limiter.NewHostRateLimiter(rateLimitConfig.ExternalAPIInterval)
	rateLimiterPort := rate_limiter_gateway.NewRateLimiterGateway(rateLimiter)

	// Create the concrete gateway implementations with rate limiting
	feedFetcherGatewayImpl := fetch_feed_gateway.NewSingleFeedGatewayWithRateLimiter(pool, rateLimiter)
	fetchFeedsListGatewayImpl := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(pool, rateLimiter)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)
	fetchFeedsListUsecase := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGatewayImpl)
	fetchFeedsListCursorUsecase := fetch_feed_usecase.NewFetchFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	fetchUnreadFeedsListCursorUsecase := fetch_feed_usecase.NewFetchUnreadFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	fetchReadFeedsListCursorUsecase := fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(fetchFeedsListGatewayImpl)
	fetchFavoriteFeedsListCursorUsecase := fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(fetchFeedsListGatewayImpl)

	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(pool)
	registerFavoriteFeedGatewayImpl := register_favorite_feed_gateway.NewRegisterFavoriteFeedGateway(pool)
	fetchFeedsGatewayImpl := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(pool, rateLimiter)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(registerFeedLinkGatewayImpl, registerFeedsGatewayImpl, fetchFeedsGatewayImpl)
	registerFeedsUsecase.SetFeedLinkIDResolver(altDBRepository)
	registerFavoriteFeedUsecase := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(registerFavoriteFeedGatewayImpl)
	feedLinkGatewayImpl := feed_link_gateway.NewFeedLinkGateway(pool)
	listFeedLinksUsecase := feed_link_usecase.NewListFeedLinksUsecase(feedLinkGatewayImpl)
	deleteFeedLinkUsecase := feed_link_usecase.NewDeleteFeedLinkUsecase(feedLinkGatewayImpl)

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
	fetchArticlesCursorUsecase := fetch_articles_usecase.NewFetchArticlesCursorUsecase(fetchArticlesGatewayImpl)

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
	recapGateway := recap_gateway.NewRecapGateway()
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
	// Image CDNs are designed for high throughput; use a dedicated rate limiter
	// with a shorter interval (3s) instead of sharing the RSS feed limiter (10s).
	imageProxyRateLimiter := rate_limiter.NewHostRateLimiter(3 * time.Second)
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
	kratosClientImpl := kratos_client.NewKratosClient(cfg.AuthHub.URL, cfg.Auth.SharedSecret)

	// Random subscription components (for Tag Trail feature)
	fetchRandomSubscriptionGatewayImpl := fetch_random_subscription_gateway.NewFetchRandomSubscriptionGateway(altDBRepository)
	fetchRandomSubscriptionUsecase := fetch_random_subscription_usecase.NewFetchRandomSubscriptionUsecase(fetchRandomSubscriptionGatewayImpl)

	// Articles by tag components (for Tag Trail feature)
	fetchArticlesByTagGatewayImpl := fetch_articles_by_tag_gateway.NewFetchArticlesByTagGateway(altDBRepository)
	fetchArticlesByTagUsecase := fetch_articles_by_tag_usecase.NewFetchArticlesByTagUsecase(fetchArticlesByTagGatewayImpl)

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
		RagIntegration: ragAdapterImpl,
		RagConnectClient: ragConnectClient,
		StreamChatPort:   morningLetterConnectGateway,
		EventPublisher:   eventPublisherGatewayImpl,

		// Usecases
		FetchSingleFeedUsecase:              fetchSingleFeedUsecase,
		FetchFeedsListUsecase:               fetchFeedsListUsecase,
		FetchFeedsListCursorUsecase:         fetchFeedsListCursorUsecase,
		FetchUnreadFeedsListCursorUsecase:   fetchUnreadFeedsListCursorUsecase,
		FetchReadFeedsListCursorUsecase:     fetchReadFeedsListCursorUsecase,
		FetchFavoriteFeedsListCursorUsecase: fetchFavoriteFeedsListCursorUsecase,
		RegisterFeedsUsecase:                registerFeedsUsecase,
		RegisterFavoriteFeedUsecase:         registerFavoriteFeedUsecase,
		ListFeedLinksUsecase:                listFeedLinksUsecase,
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

		DashboardMetricsUsecase:             dashboardMetricsUsecase,
		FetchLatestArticleUsecase:           fetchLatestArticleUsecase,
		FetchArticleSummaryUsecase:          fetchArticleSummaryUsecase,
		StreamArticleTagsUsecase:            streamArticleTagsUsecase,

		// Image Proxy
		ImageProxyUsecase:                   imageProxyUsecaseInstance,

		// Internal API gateway
		InternalArticleGateway:              internalArticleGatewayImpl,
	}
}
