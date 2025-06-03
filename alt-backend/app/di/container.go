package di

import (
	"alt/driver/alt_db"
	"alt/gateway/fetch_feed_gateway"
	"alt/gateway/register_feed_gateway"
	"alt/port/fetch_feed_port"
	"alt/port/register_feed_port"
	"alt/usecase/fetch_feed_usecase"
	"alt/usecase/register_feed_usecase.go"

	"github.com/jackc/pgx/v5"
)

type ApplicationComponents struct {
	FetchSingleFeedUsecase *fetch_feed_usecase.FetchSingleFeedUsecase
	RegisterFeedUsecase    *register_feed_usecase.RegisterFeedUsecase
	AltDBRepository        *alt_db.AltDBRepository
}

func NewApplicationComponents(db *pgx.Conn) *ApplicationComponents {
	var feedFetcherGateway fetch_feed_port.FetchSingleFeedPort
	var registerFeedGateway register_feed_port.RegisterFeedPort

	feedFetcherGatewayImpl := fetch_feed_gateway.NewFetchSingleFeedGateway(feedFetcherGateway, db)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGatewayImpl)

	registerFeedGatewayImpl := register_feed_gateway.NewRegisterFeedGateway(registerFeedGateway, db)
	registerFeedUsecase := register_feed_usecase.NewRegisterFeedUsecase(registerFeedGatewayImpl)

	return &ApplicationComponents{
		FetchSingleFeedUsecase: fetchSingleFeedUsecase,
		RegisterFeedUsecase:    registerFeedUsecase,
		AltDBRepository:        alt_db.NewAltDBRepository(db),
	}
}
