package trend_stats_gateway

import (
	"context"

	"alt/orchestrator/port/trend_stats_port"
	"alt/shared/gateway/datahub_gateway"
)

// trendStatsClient is the slice of the alt-data-hub stats gateway this package
// adapts (capability catalog §2.M W3-M6).
type trendStatsClient interface {
	TrendStats(ctx context.Context, window string) (*datahub_gateway.TrendSeries, error)
}

var _ trendStatsClient = (*datahub_gateway.StatsGateway)(nil)

// TrendStatsGateway implements trend_stats_port.TrendStatsPort.
type TrendStatsGateway struct {
	stats trendStatsClient
}

func NewTrendStatsGateway(stats trendStatsClient) *TrendStatsGateway {
	return &TrendStatsGateway{stats: stats}
}

// Execute fetches the dashboard series for a window.
//
// The mapping below is a field-for-field copy, and it is here rather than in
// datahub_gateway on purpose: trend_stats_port lives under orchestrator/, and a
// gateway in shared/ that returned the port's type would make alt-data-hub —
// which links that package to serve the other end — depend on the caller's port
// layer. The window is put back onto the response here too, because it is this
// side's own input and the provider never restates it.
func (g *TrendStatsGateway) Execute(ctx context.Context, window string) (*trend_stats_port.TrendDataResponse, error) {
	series, err := g.stats.TrendStats(ctx, window)
	if err != nil {
		return nil, err
	}

	points := make([]trend_stats_port.TrendDataPoint, 0, len(series.Points))
	for _, p := range series.Points {
		points = append(points, trend_stats_port.TrendDataPoint{
			Timestamp:    p.Timestamp,
			Articles:     p.Articles,
			Summarized:   p.Summarized,
			FeedActivity: p.FeedActivity,
		})
	}

	return &trend_stats_port.TrendDataResponse{
		DataPoints:  points,
		Granularity: series.Granularity,
		Window:      window,
	}, nil
}
