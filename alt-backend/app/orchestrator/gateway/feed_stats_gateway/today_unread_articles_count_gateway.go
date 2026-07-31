package feed_stats_gateway

import (
	"context"
	"time"
)

// TodayUnreadArticlesCountGateway implements
// feed_stats_port.TodayUnreadArticlesCountPort.
type TodayUnreadArticlesCountGateway struct {
	stats statsClient
}

func NewTodayUnreadArticlesCountGateway(stats statsClient) *TodayUnreadArticlesCountGateway {
	return &TodayUnreadArticlesCountGateway{stats: stats}
}

// Execute counts feeds newer than `since` the user has not read. `since` stays
// the caller's: "today" needs the reader's timezone, which the provider has no
// way to know.
func (g *TodayUnreadArticlesCountGateway) Execute(ctx context.Context, since time.Time) (int, error) {
	return g.stats.TodayUnreadArticlesCount(ctx, since)
}
