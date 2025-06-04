package di

import (
	"alt/driver/alt_db"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/register_feed_usecase.go"

	"github.com/jackc/pgx/v5"
)

type ApplicationComponents struct {
	FetchSingleFeedUsecase *fetch_feed_usecase.FetchSingleFeedUsecase
	FetchFeedsListUsecase  *fetch_feed_usecase.FetchFeedsListUsecase
	RegisterFeedsUsecase   *register_feed_usecase.RegisterFeedsUsecase
	AltDBRepository        *alt_db.AltDBRepository
}

func NewApplicationComponents(db *pgx.Conn) *ApplicationComponents {
	// Create the concrete gateway implementations
	feedFetcherGatewayImpl := fetch_feed_gateway.NewFetchSingleFeedGateway(db)
	fetchFeedsListGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(db)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)
	fetchFeedsListUsecase := fetch_feed_usecase.NewFetchFeedsListUsecase(fetchFeedsListGatewayImpl)

	registerFeedLinkGatewayImpl := register_feed_gateway.NewRegisterFeedLinkGateway(db)
	registerFeedsGatewayImpl := register_feed_gateway.NewRegisterFeedsGateway(db)
	fetchFeedsGatewayImpl := fetch_feed_gateway.NewFetchFeedsGateway(db)
	registerFeedsUsecase := register_feed_usecase.NewRegisterFeedsUsecase(registerFeedLinkGatewayImpl, registerFeedsGatewayImpl, fetchFeedsGatewayImpl)

	return &ApplicationComponents{
		FetchSingleFeedUsecase: fetchSingleFeedUsecase,
		FetchFeedsListUsecase:  fetchFeedsListUsecase,
		RegisterFeedsUsecase:   registerFeedsUsecase,
		AltDBRepository:        alt_db.NewAltDBRepository(db),
	}
}
