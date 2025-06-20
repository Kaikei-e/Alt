package di

import (
	"alt/driver/alt_db"
	"alt/driver/search_indexer"
	"alt/gateway/feed_search_gateway"
	"alt/gateway/feed_stats_gateway"
	"alt/gateway/feed_url_link_gateway"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_stats_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/register_feed_usecase.go"
	"alt/usecase/search_feed_usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationComponents struct {
	AltDBRepository             *alt_db.AltDBRepository
	FetchSingleFeedUsecase      *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase       *fetch_feed_usecase.FetchFeedsListUsecase
	FetchFeedsListCursorUsecase *fetch_feed_usecase.FetchFeedsListCursorUsecase
	RegisterFeedsUsecase        *register_feed_usecase.RegisterFeedsUsecase
	FeedsReadingStatusUsecase   *reading_status.FeedsReadingStatusUsecase
	FeedsSummaryUsecase         *fetch_feed_details_usecase.FeedsSummaryUsecase
	FeedAmountUsecase           *fetch_feed_stats_usecase.FeedsCountUsecase
	FeedSearchUsecase           *search_feed_usecase.SearchFeedMeilisearchUsecase
}

func NewApplicationComponents(pool *pgxpool.Pool) *ApplicationComponents {
	altDBRepository := alt_db.NewAltDBRepository(pool)

	// Create the concrete gateway implementations
	feedFetcherGatewayImpl := fetch_feed_gateway.NewSingleFeedGateway(pool)
	fetchFeedsListGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(pool)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)
	fetchFeedsListUsecase := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGatewayImpl)
	fetchFeedsListCursorUsecase := fetch_feed_usecase.NewFetchFeedsListCursorUsecase(fetchFeedsListGatewayImpl)

	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(pool)
	fetchFeedsGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(pool)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(registerFeedLinkGatewayImpl, registerFeedsGatewayImpl, fetchFeedsGatewayImpl)

	updateFeedStatusGatewayImpl := update_feed_status_gateway.NewUpdateFeedStatusGateway(pool)
	feedsReadingStatusUsecase := reading_status.NewFeedsReadingStatusUsecase(updateFeedStatusGatewayImpl)

	feedSummaryGatewayImpl := fetch_feed_detail_gateway.NewFeedSummaryGateway(pool)
	feedsSummaryUsecase := fetch_feed_details_usecase.NewFeedsSummaryUsecase(feedSummaryGatewayImpl)

	feedAmountGatewayImpl := feed_stats_gateway.NewFeedAmountGateway(pool)
	feedsCountUsecase := fetch_feed_stats_usecase.NewFeedsCountUsecase(feedAmountGatewayImpl)

	searchIndexerDriverImpl := search_indexer.NewHTTPSearchIndexerDriver()
	feedSearchMeilisearchGatewayImpl := feed_search_gateway.NewSearchFeedMeilisearchGateway(searchIndexerDriverImpl)
	feedURLLinkGatewayImpl := feed_url_link_gateway.NewFeedURLLinkGateway(altDBRepository)
	feedSearchUsecase := search_feed_usecase.NewSearchFeedMeilisearchUsecase(feedSearchMeilisearchGatewayImpl, feedURLLinkGatewayImpl)

	return &ApplicationComponents{
		AltDBRepository:             altDBRepository,
		FetchSingleFeedUsecase:      fetchSingleFeedUsecase,
		FetchFeedsListUsecase:       fetchFeedsListUsecase,
		FetchFeedsListCursorUsecase: fetchFeedsListCursorUsecase,
		RegisterFeedsUsecase:        registerFeedsUsecase,
		FeedsReadingStatusUsecase:   feedsReadingStatusUsecase,
		FeedsSummaryUsecase:         feedsSummaryUsecase,
		FeedAmountUsecase:           feedsCountUsecase,
		FeedSearchUsecase:           feedSearchUsecase,
	}
}
