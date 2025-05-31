package di

import (
	"alt/gateway/fetch_feed_gateway"
	"alt/port/fetch_feed_port"
	"alt/usecase/fetch_feed_usecase"

	"github.com/jackc/pgx/v5"
)

type ApplicationComponents struct {
	FetchSingleFeedUsecase fetch_feed_usecase.FetchSingleFeedUsecase
}

func NewApplicationComponents(db *pgx.Conn) *ApplicationComponents {
	var feedFetcherGateway fetch_feed_port.FetchSingleFeedPort

	feedFetcherGateway = fetch_feed_gateway.NewFetchSingleFeedGateway(feedFetcherGateway, db)
	fetchSingleFeedUsecase := fetch_feed_usecase.NewFetchSingleFeedUsecase(feedFetcherGateway)

	return &ApplicationComponents{
		FetchSingleFeedUsecase: *fetchSingleFeedUsecase,
	}
}
