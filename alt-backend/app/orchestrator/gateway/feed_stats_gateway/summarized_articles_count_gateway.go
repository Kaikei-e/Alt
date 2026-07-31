package feed_stats_gateway

import (
	"context"
)

// SummarizedArticlesCountGateway implements
// feed_stats_port.SummarizedArticlesCountPort.
type SummarizedArticlesCountGateway struct {
	stats statsClient
}

func NewSummarizedArticlesCountGateway(stats statsClient) *SummarizedArticlesCountGateway {
	return &SummarizedArticlesCountGateway{stats: stats}
}

func (g *SummarizedArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	return g.stats.SummarizedArticlesCount(ctx)
}
