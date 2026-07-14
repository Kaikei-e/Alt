package trend_stats_port

import (
	"context"
	"time"
)

// TrendDataPoint represents a single data point in the trend chart
type TrendDataPoint struct {
	Timestamp    time.Time
	Articles     int
	Summarized   int
	FeedActivity int
}

// TrendDataResponse represents the complete trend data response
type TrendDataResponse struct {
	DataPoints  []TrendDataPoint
	Granularity string // "hourly" or "daily"
	Window      string // "4h", "24h", "3d", "7d"
}

// TrendStatsPort defines the interface for fetching trend statistics
type TrendStatsPort interface {
	Execute(ctx context.Context, window string) (*TrendDataResponse, error)
}
