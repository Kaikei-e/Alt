package fetch_feed_gateway

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"database/sql"
)

type FetchSingleFeedGateway struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
	db                  *sql.DB
}

func NewFetchSingleFeedGateway(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort, db *sql.DB) *FetchSingleFeedGateway {
	return &FetchSingleFeedGateway{
		fetchSingleFeedPort: fetchSingleFeedPort,
		db:                  db,
	}
}

func (g *FetchSingleFeedGateway) FetchSingleFeed() (*domain.RSSFeed, error) {
	return g.fetchSingleFeedPort.FetchSingleFeed()
}
