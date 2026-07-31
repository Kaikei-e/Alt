package di

import (
	"alt/orchestrator/gateway/feed_link_gateway"
	"alt/orchestrator/gateway/feed_page_cache_gateway"
	"alt/orchestrator/gateway/feed_search_gateway"
	"alt/orchestrator/gateway/feed_stats_gateway"
	"alt/orchestrator/gateway/feed_url_link_gateway"
	"alt/orchestrator/gateway/feed_url_to_id_gateway"
	"alt/orchestrator/gateway/fetch_feed_detail_gateway"
	"alt/orchestrator/gateway/fetch_feed_gateway"
	"alt/orchestrator/gateway/fetch_feed_tags_gateway"
	"alt/orchestrator/gateway/fetch_inoreader_summary_gateway"
	"alt/orchestrator/gateway/fetch_random_subscription_gateway"
	"alt/orchestrator/gateway/register_favorite_feed_gateway"
	"alt/orchestrator/gateway/register_feed_gateway"
	"alt/orchestrator/gateway/trend_stats_gateway"
	"alt/orchestrator/gateway/validate_fetch_rss_gateway"
	"alt/orchestrator/port/scraping_domain_port"
	"alt/orchestrator/usecase/cached_feed_list_usecase"
	"alt/orchestrator/usecase/feed_link_usecase"
	"alt/orchestrator/usecase/fetch_feed_details_usecase"
	"alt/orchestrator/usecase/fetch_feed_stats_usecase"
	"alt/orchestrator/usecase/fetch_feed_tags_by_id_usecase"
	"alt/orchestrator/usecase/fetch_feed_tags_usecase"
	"alt/orchestrator/usecase/fetch_feed_usecase"
	"alt/orchestrator/usecase/fetch_inoreader_summary_usecase"
	"alt/orchestrator/usecase/fetch_random_subscription_usecase"
	"alt/orchestrator/usecase/fetch_trend_stats_usecase"
	"alt/orchestrator/usecase/reading_status"
	"alt/orchestrator/usecase/register_favorite_feed_usecase"
	"alt/orchestrator/usecase/register_feed_usecase"
	"alt/orchestrator/usecase/remove_favorite_feed_usecase"
	"alt/orchestrator/usecase/scraping_domain_usecase"
	"alt/orchestrator/usecase/search_feed_usecase"

	"alt/orchestrator/gateway/feed_link_domain_gateway"
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
	FetchFeedTagsByIDUsecase            *fetch_feed_tags_by_id_usecase.FetchFeedTagsByIDUsecase
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
	// The feed tables moved to alt-data-hub in ADR-000954 Wave 3 batch 3
	// (catalog §2.F / §2.G / §2.H). altDB stays for the read/subscription
	// state and the tag reads, which are later batches.
	feedGw := infra.FeedGateway
	feedLinkGw := infra.FeedLinkGateway

	// Feed fetch gateways. The RSS fetch each performs afterwards is external
	// HTTP and stays here (ADR-000954 D4); only the reads crossed.
	feedFetcherGw := fetch_feed_gateway.NewSingleFeedGatewayWithRateLimiter(feedLinkGw, infra.RateLimiter)
	fetchFeedsListGw := fetch_feed_gateway.NewFetchFeedsGatewayWithRateLimiter(feedGw, infra.RateLimiter)
	feedPageCacheGw := feed_page_cache_gateway.NewGateway(feedGw)

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
	registerFeedLinkGw := register_feed_gateway.NewRegisterFeedLinkGateway(feedLinkGw)
	registerFeedsGw := register_feed_gateway.NewRegisterFeedsGateway(feedGw)
	registerFavoriteFeedGw := register_favorite_feed_gateway.NewRegisterFavoriteFeedGateway(infra.ReadStateGateway)
	registerFeedsUC := register_feed_usecase.NewRegisterFeedsUsecase(validateAndFetchRSSGw, registerFeedLinkGw, registerFeedsGw, &register_feed_usecase.RegisterFeedsOpts{
		FeedLinkIDResolver:   feedLinkGw,
		FeedLinkAvailability: infra.FeedLinkAvailabilityGateway,
		FeedPageInvalidator:  feedPageCacheGw,
		SubscriptionPort:     sub.SubscriptionGateway,
	})
	registerFavoriteFeedUC := register_favorite_feed_usecase.NewRegisterFavoriteFeedUsecase(registerFavoriteFeedGw)
	removeFavoriteFeedUC := remove_favorite_feed_usecase.NewRemoveFavoriteFeedUsecase(registerFavoriteFeedGw)

	// Feed link gateways / usecases
	feedLinkMgmtGw := feed_link_gateway.NewFeedLinkGateway(feedLinkGw)
	listFeedLinksUC := feed_link_usecase.NewListFeedLinksUsecase(feedLinkMgmtGw)
	listFeedLinksWithHealthUC := feed_link_usecase.NewListFeedLinksWithHealthUsecase(feedLinkMgmtGw)

	// Reading status (capability catalog §2.I W3-I1 / W3-I2). Both writes go
	// to the same gateway: they hit one table and, since §4-5, answer an
	// absent feed the same way.
	feedsReadingStatusUC := reading_status.NewFeedsReadingStatusUsecase(infra.ReadStateGateway)
	articlesReadingStatusUC := reading_status.NewArticlesReadingStatusUsecase(infra.ReadStateGateway)

	// Feed details / stats
	feedSummaryGw := fetch_feed_detail_gateway.NewFeedSummaryGateway(feedGw)
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
	feedURLLinkGw := feed_url_link_gateway.NewFeedURLLinkGateway(feedGw)
	feedSearchUC := search_feed_usecase.NewSearchFeedMeilisearchUsecase(searchFeedMeilisearchGw, feedURLLinkGw)

	// Feed tags
	feedURLToIDGw := feed_url_to_id_gateway.NewFeedURLToIDGateway(altDB)
	fetchFeedTagsGw := fetch_feed_tags_gateway.NewFetchFeedTagsGateway(infra.TagGateway)
	fetchFeedTagsUC := fetch_feed_tags_usecase.NewFetchFeedTagsUsecase(feedURLToIDGw, fetchFeedTagsGw)
	fetchFeedTagsByIDUC := fetch_feed_tags_by_id_usecase.NewFetchFeedTagsByIDUsecase(fetchFeedTagsGw)

	// Inoreader summary
	fetchInoreaderSummaryGw := fetch_inoreader_summary_gateway.NewInoreaderSummaryGateway(feedGw)
	fetchInoreaderSummaryUC := fetch_inoreader_summary_usecase.NewFetchInoreaderSummaryUsecase(fetchInoreaderSummaryGw)

	// Random subscription (Tag Trail feature)
	fetchRandomSubscriptionGw := fetch_random_subscription_gateway.NewFetchRandomSubscriptionGateway(feedGw)
	fetchRandomSubscriptionUC := fetch_random_subscription_usecase.NewFetchRandomSubscriptionUsecase(fetchRandomSubscriptionGw)

	// Scraping domain. The recorded policy lives in alt-data-hub since
	// ADR-000954 Wave 3 (catalog §2.L); the robots.txt fetch behind
	// infra.RobotsTxtGateway is external HTTP and stays here (D4).
	scrapingDomainGw := infra.ScrapingDomainGateway
	feedLinkDomainGw := feed_link_domain_gateway.NewFeedLinkDomainGateway(feedLinkGw)
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
		ListFeedLinksWithHealthUsecase:      listFeedLinksWithHealthUC,
		// Taken straight from the subscription module rather than patched in
		// by the composition root afterwards. The old "return nil, the root
		// fills it in" shape compiled fine if a root forgot the assignment and
		// only surfaced as a nil dereference inside the RSS DeleteFeedLink
		// handler — and there are three roots now (CLAUDE.md rule 8).
		DeleteFeedLinkUsecase:            sub.DeleteFeedLinkUsecase,
		FeedsReadingStatusUsecase:        feedsReadingStatusUC,
		ArticlesReadingStatusUsecase:     articlesReadingStatusUC,
		FeedsSummaryUsecase:              feedsSummaryUC,
		FeedAmountUsecase:                feedsCountUC,
		UnsummarizedArticlesCountUsecase: unsummarizedUC,
		SummarizedArticlesCountUsecase:   summarizedUC,
		TotalArticlesCountUsecase:        totalUC,
		TodayUnreadArticlesCountUsecase:  todayUnreadUC,
		TrendStatsUsecase:                trendStatsUC,
		FeedSearchUsecase:                feedSearchUC,
		FetchFeedTagsUsecase:             fetchFeedTagsUC,
		FetchFeedTagsByIDUsecase:         fetchFeedTagsByIDUC,
		FetchInoreaderSummaryUsecase:     fetchInoreaderSummaryUC,
		FetchRandomSubscriptionUsecase:   fetchRandomSubscriptionUC,
		ScrapingDomainUsecase:            scrapingDomainUC,

		FeedPageCacheGateway:         feedPageCacheGw,
		FetchFeedsListGateway:        fetchFeedsListGw,
		SearchFeedMeilisearchGateway: searchFeedMeilisearchGw,
		ScrapingDomainGateway:        scrapingDomainGw,
	}
}
