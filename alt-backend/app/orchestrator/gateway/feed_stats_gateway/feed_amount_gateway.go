package feed_stats_gateway

import (
	"context"
	"time"

	"alt/shared/gateway/datahub_gateway"
)

// statsClient is the slice of the alt-data-hub stats gateway this package
// adapts (capability catalog §2.M).
//
// Five types in this package rather than one, because each satisfies a port
// that declares a method called Execute and no single object can answer five of
// those. What they no longer hold is a database: the query, the tenancy and the
// error wording live behind this interface, and these types are the port-shaped
// faces of one shared gateway.
//
// The `if repository == nil` guard each of them used to open with is gone along
// with the repository. It reported "database connection not available" at call
// time for a mistake made at wiring time; datahub_gateway.NewStatsGateway
// panics at construction instead, so the same mistake is now a process that
// does not start rather than a dashboard of zeroes (CLAUDE.md rule 8).
type statsClient interface {
	FeedAmount(ctx context.Context) (int, error)
	TotalArticlesCount(ctx context.Context) (int, error)
	SummarizedArticlesCount(ctx context.Context) (int, error)
	UnsummarizedArticlesCount(ctx context.Context) (int, error)
	TodayUnreadArticlesCount(ctx context.Context, since time.Time) (int, error)
}

var _ statsClient = (*datahub_gateway.StatsGateway)(nil)

// FeedAmountGateway implements feed_stats_port.FeedAmountPort.
type FeedAmountGateway struct {
	stats statsClient
}

func NewFeedAmountGateway(stats statsClient) *FeedAmountGateway {
	return &FeedAmountGateway{stats: stats}
}

// Execute counts every feed in the deployment — the one dashboard number that
// is not per-user.
func (g *FeedAmountGateway) Execute(ctx context.Context) (int, error) {
	return g.stats.FeedAmount(ctx)
}
