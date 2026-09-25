package datahub_capability_port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TrendPoint is one time bucket of the dashboard chart.
//
// Declared here rather than borrowed from orchestrator/port/trend_stats_port,
// which is the caller's port and now lives in another process. The two are
// field-for-field identical today; keeping them separate is what stops the
// data plane's contract from being defined by whoever renders the chart.
type TrendPoint struct {
	Timestamp    time.Time
	Articles     int
	Summarized   int
	FeedActivity int
}

// TrendSeries is one window of the dashboard chart as the provider computed it.
type TrendSeries struct {
	Points []TrendPoint
	// "hourly" or "daily". Reported rather than requested: which unit a window
	// groups by is a property of the query.
	Granularity string
}

// StatsPort is the dashboard's counts and series (catalog §2.M).
//
// userID is an argument on every method but FeedAmount, and FeedAmount has none
// because it counts the whole deployment. The in-process drivers read the user
// from the request context; that could not survive the move, because the
// context alt-data-hub has describes a peer certificate naming alt-backend.
type StatsPort interface {
	// FeedAmount counts every feed row. The one unscoped read here.
	FeedAmount(ctx context.Context) (int, error)
	TotalArticles(ctx context.Context, userID uuid.UUID) (int, error)
	SummarizedArticles(ctx context.Context, userID uuid.UUID) (int, error)
	// UnsummarizedArticles is its own query rather than a subtraction: a
	// summary can outlive the article it describes.
	UnsummarizedArticles(ctx context.Context, userID uuid.UUID) (int, error)
	// TodayUnread counts feeds newer than `since` the user has not read.
	// `since` is the caller's, because "today" needs the reader's timezone.
	TodayUnread(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
	// TrendStats rejects a window outside the closed set the query plans for.
	TrendStats(ctx context.Context, userID uuid.UUID, window string) (*TrendSeries, error)
	// UserFeedIDs returns the feeds the user has read state against.
	UserFeedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}
