package di

import (
	"alt/driver/alt_db"
	"alt/gateway/fetch_feed_detail_gateway"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/gateway/update_feed_status_gateway"
	"alt/usecase/fetch_feed_details_usecase"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/reading_status"
	"alt/usecase/register_feed_usecase.go"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationComponents struct {
	FetchSingleFeedUsecase    *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase     *fetch_feed_usecase.FetchFeedsListUsecase
	RegisterFeedsUsecase      *register_feed_usecase.RegisterFeedsUsecase
	FeedsReadingStatusUsecase *reading_status.FeedsReadingStatusUsecase
	FeedsSummaryUsecase       *fetch_feed_details_usecase.FeedsSummaryUsecase
	AltDBRepository           *alt_db.AltDBRepository
}

func NewApplicationComponents(pool *pgxpool.Pool) *ApplicationComponents {
	// Create the concrete gateway implementations
	feedFetcherGatewayImpl := fetch_feed_gateway.NewFetchSingleFeedGateway(pool)
	fetchFeedsListGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(pool)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)
	fetchFeedsListUsecase := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGatewayImpl)

	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(pool)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(pool)
	fetchFeedsGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(pool)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(registerFeedLinkGatewayImpl, registerFeedsGatewayImpl, fetchFeedsGatewayImpl)

	updateFeedStatusGatewayImpl := update_feed_status_gateway.NewUpdateFeedStatusGateway(pool)
	feedsReadingStatusUsecase := reading_status.NewFeedsReadingStatusUsecase(updateFeedStatusGatewayImpl)

	feedSummaryGatewayImpl := fetch_feed_detail_gateway.NewFeedSummaryGateway(pool)
	feedsSummaryUsecase := fetch_feed_details_usecase.NewFeedsSummaryUsecase(feedSummaryGatewayImpl)

	altDBRepository := alt_db.NewAltDBRepository(pool)

	return &ApplicationComponents{
		FetchSingleFeedUsecase:    fetchSingleFeedUsecase,
		FetchFeedsListUsecase:     fetchFeedsListUsecase,
		RegisterFeedsUsecase:      registerFeedsUsecase,
		FeedsReadingStatusUsecase: feedsReadingStatusUsecase,
		FeedsSummaryUsecase:       feedsSummaryUsecase,
		AltDBRepository:           altDBRepository,
	}
}
