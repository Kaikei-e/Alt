package di

import (
	"alt/config"
	"alt/driver/alt_db"
	"alt/driver/search_indexer"
	"alt/gateway/config_gateway"
	"alt/gateway/error_handler_gateway"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_link_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/rate_limiter_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/port/config_port"
	"alt/port/rate_limiter_port"
	"alt/port/error_handler_port"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/register_feed_usecase.go"
	"alt/usecase/search_feed_usecase"
	"alt/utils/rate_limiter"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationComponents struct {
	// Ports
	ConfigPort      config_port.ConfigPort
	RateLimiterPort rate_limiter_port.RateLimiterPort
	ErrorHandlerPort error_handler_port.ErrorHandlerPort
	
	// Repository
	AltDBRepository                   *alt_db.AltDBRepository
	
	// Usecases
	FetchSingleFeedUsecase            *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase             *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase       *fetch_feed_usecase.FetchFeedsListCursorUsecase
	RegisterFeedsUsecase              *register_feed_usecase.RegisterFeedsUsecase
	FeedsReadingStatusUsecase         *reading_status.FeedsReadingStatusUsecase
	FeedsSummaryUsecase               *fetch_feed_details_usecase.FeedsSummaryUsecase
	FeedAmountUsecase                 *fetch_feed_stats_usecase.FeedsCountUsecase
	UnsummarizedArticlesCountUsecase  *fetch_feed_stats_usecase.UnsummarizedArticlesCountUsecase
	SummarizedArticlesCountUsecase    *fetch_feed_stats_usecase.SummarizedArticlesCountUsecase
	TotalArticlesCountUsecase         *fetch_feed_stats_usecase.TotalArticlesCountUsecase
	FeedSearchUsecase                 *search_feed_usecase.SearchFeedMeilisearchUsecase
}

func NewApplicationComponents(pool *pgxpool.Pool) *ApplicationComponents {
	altDBRepository := alt_db.NewAltDBRepository(pool)

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

	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(pool)
	fetchFeedsGatewayImpl := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(pool, rateLimiter)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(registerFeedLinkGatewayImpl, registerFeedsGatewayImpl, fetchFeedsGatewayImpl)

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

	searchIndexerDriverImpl := search_indexer.NewHTTPSearchIndexerDriver()
	feedSearchMeilisearchGatewayImpl := feed_search_gateway.NewSearchFeedMeilisearchGateway(searchIndexerDriverImpl)
	feedURLLinkGatewayImpl := feed_url_link_gateway.NewFeedURLLinkGateway(altDBRepository)
	feedSearchUsecase := search_feed_usecase.NewSearchFeedMeilisearchUsecase(feedSearchMeilisearchGatewayImpl, feedURLLinkGatewayImpl)

	return &ApplicationComponents{
		// Ports
		ConfigPort:       configPort,
		RateLimiterPort:  rateLimiterPort,
		ErrorHandlerPort: errorHandlerPort,
		
		// Repository
		AltDBRepository:                  altDBRepository,
		
		// Usecases
		FetchSingleFeedUsecase:           fetchSingleFeedUsecase,
		FetchFeedsListUsecase:            fetchFeedsListUsecase,
		FetchFeedsListCursorUsecase:      fetchFeedsListCursorUsecase,
		RegisterFeedsUsecase:             registerFeedsUsecase,
		FeedsReadingStatusUsecase:        feedsReadingStatusUsecase,
		FeedsSummaryUsecase:              feedsSummaryUsecase,
		FeedAmountUsecase:                feedsCountUsecase,
		UnsummarizedArticlesCountUsecase: unsummarizedArticlesCountUsecase,
		SummarizedArticlesCountUsecase:   summarizedArticlesCountUsecase,
		TotalArticlesCountUsecase:        totalArticlesCountUsecase,
		FeedSearchUsecase:                feedSearchUsecase,
	}
}
