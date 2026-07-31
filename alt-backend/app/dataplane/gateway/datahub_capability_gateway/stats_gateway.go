package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// statsDriver is the slice of alt_db the dashboard capabilities use.
//
// Every method but FetchFeedAmount takes the owner as an argument. Those
// drivers used to read it from the request context; ADR-000954 Wave 3 batch 5
// gave them explicit-owner forms, because the context alt-data-hub serves these
// under describes a peer certificate naming alt-backend and knows nothing about
// whose dashboard is being drawn.
type statsDriver interface {
	FetchFeedAmount(ctx context.Context) (int, error)
	FetchTotalArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error)
	FetchSummarizedArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error)
	FetchUnsummarizedArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error)
	FetchTodayUnreadArticlesCountForUser(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
	FetchTrendStatsForUser(ctx context.Context, userID uuid.UUID, window string) (*alt_db.TrendStats, error)
	FetchUserFeedIDsForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}

// StatsGateway implements datahub_capability_port.StatsPort.
type StatsGateway struct {
	db statsDriver
}

func NewStatsGateway(db *alt_db.AltDBRepository) *StatsGateway {
	return &StatsGateway{db: db}
}

func (g *StatsGateway) FeedAmount(ctx context.Context) (int, error) {
	count, err := g.db.FetchFeedAmount(ctx)
	if err != nil {
		return 0, fmt.Errorf("get feed amount: %w", err)
	}
	return count, nil
}

func (g *StatsGateway) TotalArticles(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := g.db.FetchTotalArticlesCountForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get total articles count for user %s: %w", userID, err)
	}
	return count, nil
}

func (g *StatsGateway) SummarizedArticles(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := g.db.FetchSummarizedArticlesCountForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get summarized articles count for user %s: %w", userID, err)
	}
	return count, nil
}

func (g *StatsGateway) UnsummarizedArticles(ctx context.Context, userID uuid.UUID) (int, error) {
	count, err := g.db.FetchUnsummarizedArticlesCountForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get unsummarized articles count for user %s: %w", userID, err)
	}
	return count, nil
}

func (g *StatsGateway) TodayUnread(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	count, err := g.db.FetchTodayUnreadArticlesCountForUser(ctx, userID, since)
	if err != nil {
		return 0, fmt.Errorf("get today unread articles count for user %s: %w", userID, err)
	}
	return count, nil
}

// TrendStats maps the driver's rows onto the port's shape.
//
// The window is not carried back. The driver echoed it into its response
// because the caller was in the same process and it cost nothing; across the
// boundary it would be the provider restating the caller's own input, which is
// one more place for the two to disagree.
func (g *StatsGateway) TrendStats(ctx context.Context, userID uuid.UUID, window string) (*datahub_capability_port.TrendSeries, error) {
	stats, err := g.db.FetchTrendStatsForUser(ctx, userID, window)
	if err != nil {
		return nil, fmt.Errorf("get trend stats for user %s window %s: %w", userID, window, err)
	}

	points := make([]datahub_capability_port.TrendPoint, 0, len(stats.Rows))
	for _, row := range stats.Rows {
		points = append(points, datahub_capability_port.TrendPoint{
			Timestamp:    row.Bucket,
			Articles:     row.Articles,
			Summarized:   row.Summarized,
			FeedActivity: row.FeedActivity,
		})
	}
	return &datahub_capability_port.TrendSeries{Points: points, Granularity: stats.Granularity}, nil
}

func (g *StatsGateway) UserFeedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := g.db.FetchUserFeedIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user feed ids for user %s: %w", userID, err)
	}
	return ids, nil
}
