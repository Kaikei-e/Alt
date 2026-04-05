package di

import (
	"alt/gateway/feed_link_gateway"
	"alt/gateway/feed_page_cache_gateway"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_link_gateway"
	"alt/gateway/feed_url_to_id_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/fetch_feed_tags_gateway"
	"alt/gateway/fetch_inoreader_summary_gateway"
	"alt/gateway/fetch_random_subscription_gateway"
	"alt/gateway/register_favorite_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/scraping_domain_gateway"
	"alt/port/scraping_domain_port"
	"alt/gateway/trend_stats_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/gateway/validate_fetch_rss_gateway"
	"alt/usecase/cached_feed_list_usecase"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_tags_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/fetch_inoreader_summary_usecase"
	"alt/usecase/fetch_random_subscription_usecase"
	"alt/usecase/fetch_trend_stats_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/register_favorite_feed_usecase"
	"alt/usecase/register_feed_usecase"
	"alt/usecase/remove_favorite_feed_usecase"
	"alt/usecase/scraping_domain_usecase"
	"alt/usecase/search_feed_usecase"

	"alt/gateway/feed_link_domain_gateway"
)

// FeedModule holds all feed-domain components.
type FeedModule struct {
	// Usecases
	FetchSingleFeedUsecase              *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase               *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase         *fetch_feed_usecase.FetchFeedsListCursorUsecase
	FetchUnreadFeedsListCursorUsecase   *fetch_feed_usecase.FetchUnreadFeedsListCursorUsecase
	FetchReadFeedsListCursorUsecase     *fetch_feed_usecase.FetchReadFeedsListCursorUsecase
	FetchFavoriteFeedsListCursorUsecase *fetch_feed_usecase.FetchFavoriteFeedsListCursorUsecase
	CachedFeedListUsecase               *cached_feed_list_usecase.CachedFeedListUsecase
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
	FetchFeedTagsUsecase                *fetch_feed_tags_usecase.FetchFeedTagsUsecase
	FetchInoreaderSummaryUsecase        fetch_inoreader_summary_usecase.FetchInoreaderSummaryUsecase
	FetchRandomSubscriptionUsecase      *fetch_random_subscription_usecase.FetchRandomSubscriptionUsecase
	ScrapingDomainUsecase               *scraping_domain_usecase.ScrapingDomainUsecase

	// Gateways exposed for cross-module wiring
	FeedPageCacheGateway         *feed_page_cache_gateway.Gateway
	FetchFeedsListGateway        *fetch_feed_gateway.FetchFeedsGateway
	SearchFeedMeilisearchGateway *feed_search_gateway.SearchFeedMeilisearchGateway
	ScrapingDomainGateway        scraping_domain_port.ScrapingDomainPort
}

func newFeedModule(infra *InfraModule, sub *SubscriptionModule) *FeedModule {
	pool := infra.Pool
	altDB := infra.AltDBRepository

	// Feed fetch gateways
	feedFetcherGw := fetch_feed_gateway.NewSingleFeedGatewayWithRateLimiter(pool, infra.RateLimiter)
	fetchFeedsListGw := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(pool, infra.RateLimiter)
	feedPageCacheGw := feed_page_cache_gateway.NewGateway(altDB)

	// Feed fetch usecases
	fetchSingleFeedUC := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGw)
	fetchFeedsListUC := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGw)
	fetchFeedsListCursorUC := fetch_feed_usecase.NewFetchFeedsListCursorUsecase(fetchFeedsListGw)
	fetchUnreadFeedsListCursorUC := fetch_feed_usecase.NewFetchUnreadFeedsListCursorUsecase(fetchFeedsListGw)
	cachedFeedListUC := cached_feed_list_usecase.NewCachedFeedListUsecase(fetchFeedsListGw, fetchFeedsListGw, fetchFeedsListGw)
	fetchReadFeedsListCursorUC := fetch_feed_usecase.NewFetchReadFeedsListCursorUsecase(fetchFeedsListGw)
	fetchFavoriteFeedsListCursorUC := fetch_feed_usecase.NewFetchFavoriteFeedsListCursorUsecase(fetchFeedsListGw)

	// Register feed gateways / usecases
	validateAndFetchRSSGw := validate_fetch_rss_gateway.NewValidateAndFetchRSSGateway()
	registerFeedLinkGw := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGw := register_feed_gateway.NewRegisterFeedsGateway(pool)
	registerFavoriteFeedGw := register_favorite_feed_gateway.NewRegisterFavoriteFeedGateway(pool)
	registerFeedsUC := register_feed_usecase.NewRegisterFeedsUsecase(validateAndFetchRSSGw, registerFeedLinkGw, registerFeedsGw, &register_feed_usecase.RegisterFeedsOpts{
		FeedLinkIDResolver:   altDB,
		FeedLinkAvailability: altDB,
		FeedPageInvalidator:  feedPageCacheGw,
		SubscriptionPort:     sub.SubscriptionGateway,
		EventPublisher:       infra.EventPublisher,
	})
	registerFavoriteFeedUC := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(registerFavoriteFeedGw)
	removeFavoriteFeedUC := remove_favorite_feed_usecase.NewRemoveFavoriteFeedUsecase(registerFavoriteFeedGw)

	// Feed link gateways / usecases
	feedLinkGw := feed_link_gateway.NewFeedLinkGateway(pool)
	listFeedLinksUC := feed_link_usecase.NewListFeedLinksUsecase(feedLinkGw)
	listFeedLinksWithHealthUC := feed_link_usecase.NewListFeedLinksWithHealthUsecase(feedLinkGw)

	// Reading status
	updateFeedStatusGw := update_feed_status_gateway.NewUpdateFeedStatusGateway(pool)
	feedsReadingStatusUC := reading_status.NewFeedsReadingStatusUsecase(updateFeedStatusGw)
	articlesReadingStatusUC := reading_status.NewArticlesReadingStatusUsecase(altDB)

	// Feed details / stats
	feedSummaryGw := fetch_feed_detail_gateway.NewFeedSummaryGateway(pool)
	feedsSummaryUC := fetch_feed_details_usecase.NewFeedsSummaryUsecase(feedSummaryGw)

	feedAmountGw := feed_stats_gateway.NewFeedAmountGateway(pool)
	feedsCountUC := fetch_feed_stats_usecase.NewFeedsCountUsecase(feedAmountGw)

	unsummarizedGw := feed_stats_gateway.NewUnsummarizedArticlesCountGateway(pool)
	unsummarizedUC := fetch_feed_stats_usecase.NewUnsummarizedArticlesCountUsecase(unsummarizedGw)

	summarizedGw := feed_stats_gateway.NewSummarizedArticlesCountGateway(pool)
	summarizedUC := fetch_feed_stats_usecase.NewSummarizedArticlesCountUsecase(summarizedGw)

	totalGw := feed_stats_gateway.NewTotalArticlesCountGateway(pool)
	totalUC := fetch_feed_stats_usecase.NewTotalArticlesCountUsecase(totalGw)

	todayUnreadGw := feed_stats_gateway.NewTodayUnreadArticlesCountGateway(pool)
	todayUnreadUC := fetch_feed_stats_usecase.NewTodayUnreadArticlesCountUsecase(todayUnreadGw)

	// Trend stats
	trendStatsGw := trend_stats_gateway.NewTrendStatsGateway(pool)
	trendStatsUC := fetch_trend_stats_usecase.NewFetchTrendStatsUsecase(trendStatsGw)

	// Feed search (Meilisearch-based via search-indexer)
	searchFeedMeilisearchGw := feed_search_gateway.NewSearchFeedMeilisearchGateway(infra.SearchIndexerDriver)
	feedURLLinkGw := feed_url_link_gateway.NewFeedURLLinkGateway(altDB)
	feedSearchUC := search_feed_usecase.NewSearchFeedMeilisearchUsecase(searchFeedMeilisearchGw, feedURLLinkGw)

	// Feed tags
	feedURLToIDGw := feed_url_to_id_gateway.NewFeedURLToIDGateway(altDB)
	fetchFeedTagsGw := fetch_feed_tags_gateway.NewFetchFeedTagsGateway(altDB)
	fetchFeedTagsUC := fetch_feed_tags_usecase.NewFetchFeedTagsUsecase(feedURLToIDGw, fetchFeedTagsGw)

	// Inoreader summary
	fetchInoreaderSummaryGw := fetch_inoreader_summary_gateway.NewInoreaderSummaryGateway(altDB)
	fetchInoreaderSummaryUC := fetch_inoreader_summary_usecase.NewFetchInoreaderSummaryUsecase(fetchInoreaderSummaryGw)

	// Random subscription (Tag Trail feature)
	fetchRandomSubscriptionGw := fetch_random_subscription_gateway.NewFetchRandomSubscriptionGateway(altDB)
	fetchRandomSubscriptionUC := fetch_random_subscription_usecase.NewFetchRandomSubscriptionUsecase(fetchRandomSubscriptionGw)

	// Scraping domain
	scrapingDomainGw := scraping_domain_gateway.NewScrapingDomainGateway(altDB)
	feedLinkDomainGw := feed_link_domain_gateway.NewFeedLinkDomainGateway(altDB)
	scrapingDomainUC := scraping_domain_usecase.NewScrapingDomainUsecaseWithFeedLinkDomain(scrapingDomainGw, infra.RobotsTxtGateway, feedLinkDomainGw)

	return &FeedModule{
		FetchSingleFeedUsecase:              fetchSingleFeedUC,
		FetchFeedsListUsecase:               fetchFeedsListUC,
		FetchFeedsListCursorUsecase:         fetchFeedsListCursorUC,
		FetchUnreadFeedsListCursorUsecase:   fetchUnreadFeedsListCursorUC,
		FetchReadFeedsListCursorUsecase:     fetchReadFeedsListCursorUC,
		FetchFavoriteFeedsListCursorUsecase: fetchFavoriteFeedsListCursorUC,
		CachedFeedListUsecase:               cachedFeedListUC,
		RegisterFeedsUsecase:                registerFeedsUC,
		RegisterFavoriteFeedUsecase:         registerFavoriteFeedUC,
		RemoveFavoriteFeedUsecase:           removeFavoriteFeedUC,
		ListFeedLinksUsecase:                listFeedLinksUC,
		ListFeedLinksWithHealthUsecase:       listFeedLinksWithHealthUC,
		DeleteFeedLinkUsecase:               nil, // set after subscription module
		FeedsReadingStatusUsecase:           feedsReadingStatusUC,
		ArticlesReadingStatusUsecase:        articlesReadingStatusUC,
		FeedsSummaryUsecase:                 feedsSummaryUC,
		FeedAmountUsecase:                   feedsCountUC,
		UnsummarizedArticlesCountUsecase:    unsummarizedUC,
		SummarizedArticlesCountUsecase:      summarizedUC,
		TotalArticlesCountUsecase:           totalUC,
		TodayUnreadArticlesCountUsecase:     todayUnreadUC,
		TrendStatsUsecase:                   trendStatsUC,
		FeedSearchUsecase:                   feedSearchUC,
		FetchFeedTagsUsecase:                fetchFeedTagsUC,
		FetchInoreaderSummaryUsecase:        fetchInoreaderSummaryUC,
		FetchRandomSubscriptionUsecase:      fetchRandomSubscriptionUC,
		ScrapingDomainUsecase:               scrapingDomainUC,

		FeedPageCacheGateway:         feedPageCacheGw,
		FetchFeedsListGateway:        fetchFeedsListGw,
		SearchFeedMeilisearchGateway: searchFeedMeilisearchGw,
		ScrapingDomainGateway:        scrapingDomainGw,
	}
}
