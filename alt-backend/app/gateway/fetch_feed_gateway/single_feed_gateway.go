package fetch_feed_gateway

import (
	"alt/domain"
	"alt/port/fetch_feed_port"

	"github.com/jackc/pgx/v5"
)

type FetchSingleFeedGateway struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
	db                  *pgx.Conn
}

func NewFetchSingleFeedGateway(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort, db *pgx.Conn) *FetchSingleFeedGateway {
	return &FetchSingleFeedGateway{
		fetchSingleFeedPort: fetchSingleFeedPort,
		db:                  db,
	}
}

func (g *FetchSingleFeedGateway) FetchSingleFeed() (*domain.RSSFeed, error) {
	return g.fetchSingleFeedPort.FetchSingleFeed()
}
