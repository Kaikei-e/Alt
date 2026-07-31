package feed_stats_gateway

import (
	"context"
)

// UnsummarizedArticlesCountGateway implements
// feed_stats_port.UnsummarizedArticlesCountPort.
//
// Its own call rather than a subtraction of the two counts beside it: a summary
// can outlive the article it describes, so total minus summarized is a
// different number.
type UnsummarizedArticlesCountGateway struct {
	stats statsClient
}

func NewUnsummarizedArticlesCountGateway(stats statsClient) *UnsummarizedArticlesCountGateway {
	return &UnsummarizedArticlesCountGateway{stats: stats}
}

func (g *UnsummarizedArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	return g.stats.UnsummarizedArticlesCount(ctx)
}
