package di

import (
	"alt/config"
	"alt/driver/alt_db"
	"alt/driver/csrf_token_driver"
	"alt/driver/search_indexer"
	"alt/gateway/archive_article_gateway"
	"alt/gateway/article_gateway"
	"alt/gateway/config_gateway"
	"alt/gateway/csrf_token_gateway"
	"alt/gateway/error_handler_gateway"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_to_id_gateway"
	"alt/gateway/fetch_article_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/fetch_feed_tags_gateway"
	"alt/gateway/fetch_inoreader_summary_gateway"
	"alt/gateway/image_fetch_gateway"
	"alt/gateway/rate_limiter_gateway"
	"alt/gateway/recap_articles_gateway"
	"alt/gateway/register_favorite_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/port/config_port"
	"alt/port/error_handler_port"
	"alt/port/rate_limiter_port"
	"alt/usecase/archive_article_usecase"
	"alt/usecase/csrf_token_usecase"
	"alt/usecase/fetch_article_usecase"
	"alt/usecase/fetch_articles_usecase"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_tags_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/fetch_inoreader_summary_usecase"
	"alt/usecase/image_fetch_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/recap_articles_usecase"
	"alt/usecase/register_favorite_feed_usecase"
	"alt/usecase/register_feed_usecase"
	"alt/usecase/search_article_usecase"
	"alt/usecase/search_feed_usecase"
	"alt/utils"
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

	// Usecases
	FetchSingleFeedUsecase              *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase               *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase         *fetch_feed_usecase.FetchFeedsListCursorUsecase
	FetchUnreadFeedsListCursorUsecase   *fetch_feed_usecase.FetchUnreadFeedsListCursorUsecase
	FetchReadFeedsListCursorUsecase     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsListCursorUsecase *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	RegisterFeedsUsecase                *register_feed_usecase.RegisterFeedsUsecase
	RegisterFavoriteFeedUsecase         *register_favorite_feed_usecase.RegisterFavoriteFeedUsecase
	FeedsReadingStatusUsecase           *reading_status.FeedsReadingStatusUsecase
	FeedsSummaryUsecase                 *fetch_feed_details_usecase.FeedsSummaryUsecase
	FeedAmountUsecase                   *fetch_feed_stats_usecase.FeedsCountUsecase
	UnsummarizedArticlesCountUsecase    *fetch_feed_stats_usecase.UnsummarizedArticlesCountUsecase
	SummarizedArticlesCountUsecase      *fetch_feed_stats_usecase.SummarizedArticlesCountUsecase
	TotalArticlesCountUsecase           *fetch_feed_stats_usecase.TotalArticlesCountUsecase
	TodayUnreadArticlesCountUsecase     *fetch_feed_stats_usecase.TodayUnreadArticlesCountUsecase
	FeedSearchUsecase                   *search_feed_usecase.SearchFeedByTitleUsecase
	ArticleSearchUsecase                *search_article_usecase.SearchArticleUsecase
	FetchFeedTagsUsecase                *fetch_feed_tags_usecase.FetchFeedTagsUsecase
	FetchInoreaderSummaryUsecase        fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	ImageFetchUsecase                   image_fetch_usecase.ImageFetchUsecaseInterface
	CSRFTokenUsecase                    *csrf_token_usecase.CSRFTokenUsecase
	ArticleUsecase                      fetch_article_usecase.ArticleUsecase
	ArchiveArticleUsecase               *archive_article_usecase.ArchiveArticleUsecase
	FetchArticlesCursorUsecase          *fetch_articles_usecase.FetchArticlesCursorUsecase
	RecapArticlesUsecase                *recap_articles_usecase.RecapArticlesUsecase
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
	registerFavoriteFeedUsecase := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(registerFavoriteFeedGatewayImpl)

	updateFeedStatusGatewayImpl := update_feed_status_gateway.NewUpdateFeedStatusGateway(pool)
	feedsReadingStatusUsecase := reading_status.NewFeedsReadingStatusUsecase(updateFeedStatusGatewayImpl)

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

	// Feed search by title (PostgreSQL-based)
	searchByTitleGatewayImpl := feed_search_gateway.NewSearchByTitleGateway(pool)
	feedSearchUsecase := search_feed_usecase.NewSearchFeedByTitleUsecase(searchByTitleGatewayImpl)

	// Article search (Meilisearch-based via search-indexer)
	searchIndexerDriver := search_indexer.NewHTTPSearchIndexerDriver()
	articleSearchUsecase := search_article_usecase.NewSearchArticleUsecase(searchIndexerDriver)

	feedURLToIDGatewayImpl := feed_url_to_id_gateway.NewFeedURLToIDGateway(altDBRepository)
	fetchFeedTagsGatewayImpl := fetch_feed_tags_gateway.NewFetchFeedTagsGateway(altDBRepository)
	fetchFeedTagsUsecase := fetch_feed_tags_usecase.NewFetchFeedTagsUsecase(feedURLToIDGatewayImpl, fetchFeedTagsGatewayImpl)

	fetchArticleGatewayImpl := fetch_article_gateway.NewFetchArticleGateway(rateLimiter, httpClient)
	fetchArticleUsecase := fetch_article_usecase.NewArticleUsecase(fetchArticleGatewayImpl)
	archiveArticleGatewayImpl := archive_article_gateway.NewArchiveArticleGateway(altDBRepository)
	archiveArticleUsecase := archive_article_usecase.NewArchiveArticleUsecase(fetchArticleGatewayImpl, archiveArticleGatewayImpl)

	// Fetch articles with cursor components
	fetchArticlesGatewayImpl := article_gateway.NewFetchArticlesGateway(pool)
	fetchArticlesCursorUsecase := fetch_articles_usecase.NewFetchArticlesCursorUsecase(fetchArticlesGatewayImpl)
	recapArticlesGateway := recap_articles_gateway.NewGateway(altDBRepository)
	recapUsecaseCfg := recap_articles_usecase.Config{
		DefaultPageSize: cfg.Recap.DefaultPageSize,
		MaxPageSize:     cfg.Recap.MaxPageSize,
		MaxRangeDays:    cfg.Recap.MaxRangeDays,
	}
	recapArticlesUsecase := recap_articles_usecase.NewRecapArticlesUsecase(recapArticlesGateway, recapUsecaseCfg)

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

	return &ApplicationComponents{
		// Ports
		ConfigPort:       configPort,
		RateLimiterPort:  rateLimiterPort,
		ErrorHandlerPort: errorHandlerPort,

		// Repository
		AltDBRepository: altDBRepository,

		// Usecases
		FetchSingleFeedUsecase:              fetchSingleFeedUsecase,
		FetchFeedsListUsecase:               fetchFeedsListUsecase,
		FetchFeedsListCursorUsecase:         fetchFeedsListCursorUsecase,
		FetchUnreadFeedsListCursorUsecase:   fetchUnreadFeedsListCursorUsecase,
		FetchReadFeedsListCursorUsecase:     fetchReadFeedsListCursorUsecase,
		FetchFavoriteFeedsListCursorUsecase: fetchFavoriteFeedsListCursorUsecase,
		RegisterFeedsUsecase:                registerFeedsUsecase,
		RegisterFavoriteFeedUsecase:         registerFavoriteFeedUsecase,
		FeedsReadingStatusUsecase:           feedsReadingStatusUsecase,
		FeedsSummaryUsecase:                 feedsSummaryUsecase,
		FeedAmountUsecase:                   feedsCountUsecase,
		UnsummarizedArticlesCountUsecase:    unsummarizedArticlesCountUsecase,
		SummarizedArticlesCountUsecase:      summarizedArticlesCountUsecase,
		TotalArticlesCountUsecase:           totalArticlesCountUsecase,
		TodayUnreadArticlesCountUsecase:     todayUnreadArticlesCountUsecase,
		FeedSearchUsecase:                   feedSearchUsecase,
		ArticleSearchUsecase:                articleSearchUsecase,
		FetchFeedTagsUsecase:                fetchFeedTagsUsecase,
		FetchInoreaderSummaryUsecase:        fetchInoreaderSummaryUsecase,
		ImageFetchUsecase:                   imageFetchUsecase,
		CSRFTokenUsecase:                    csrfTokenUsecase,
		ArticleUsecase:                      fetchArticleUsecase,
		ArchiveArticleUsecase:               archiveArticleUsecase,
		FetchArticlesCursorUsecase:          fetchArticlesCursorUsecase,
		RecapArticlesUsecase:                recapArticlesUsecase,
	}
}
