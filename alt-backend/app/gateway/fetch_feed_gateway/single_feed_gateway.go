package fetch_feed_gateway

import (
	"alt/port/fetch_feed_port"
)

type FetchSingleFeedGateway struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedGateway(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedGateway {
	return &FetchSingleFeedGateway{fetchSingleFeedPort: fetchSingleFeedPort}
}
