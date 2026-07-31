package feed_stats_gateway

import (
	"context"
)

// TotalArticlesCountGateway implements feed_stats_port.TotalArticlesCountPort.
type TotalArticlesCountGateway struct {
	stats statsClient
}

func NewTotalArticlesCountGateway(stats statsClient) *TotalArticlesCountGateway {
	return &TotalArticlesCountGateway{stats: stats}
}

func (g *TotalArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	return g.stats.TotalArticlesCount(ctx)
}
