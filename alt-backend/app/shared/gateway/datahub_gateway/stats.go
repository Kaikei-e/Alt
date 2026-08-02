package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// TrendPoint and TrendSeries are this package's own shape for the dashboard
// series, not trend_stats_port's.
//
// The port type lives under orchestrator/, and a gateway in shared/ that
// returned it would make every binary linking this package — including
// alt-data-hub, which serves the other end — depend on the caller's port
// layer. The thin adapter in orchestrator/gateway/trend_stats_gateway maps
// these two structs onto the port's, which is a field-for-field copy and the
// price of not pointing the dependency backwards.
type TrendPoint struct {
	Timestamp    time.Time
	Articles     int
	Summarized   int
	FeedActivity int
}

type TrendSeries struct {
	Points []TrendPoint
	// "hourly" or "daily" — the bucket size the provider chose for the
	// window, reported rather than requested.
	Granularity string
}

// StatsGateway is the dashboard's counts and series (capability catalog §2.M).
//
// One gateway, seven distinctly-named methods, rather than seven gateways each
// exposing Execute(ctx). The ports these ultimately serve all declare a method
// called Execute, so they cannot be satisfied by one type; the thin adapters in
// orchestrator/gateway/{feed_stats,trend_stats,user_feed}_gateway keep that
// shape and hold one of these. That leaves the wire mapping in a single place
// while the port surface stays exactly as the usecases declared it.
//
// Every method except FeedAmount resolves the tenant from the request context
// here, at the boundary, and sends it as a field. That is the one thing this
// layer must do that the drivers did not: alt-data-hub sees a peer certificate
// naming alt-backend, so a user read from the context on the far side would be
// the service account and every dashboard would show identical numbers.
type StatsGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewStatsGateway(client datahubv1connect.DataHubServiceClient) *StatsGateway {
	if client == nil {
		panic("datahub_gateway: StatsGateway requires a DataHubService client — " +
			"a nil client would make every dashboard count fail rather than read " +
			"zero, which is the distinction the panic exists to keep " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &StatsGateway{client: client}
}

// FeedAmount counts every feed in the deployment. The only unscoped read here,
// and therefore the only one that takes no user.
func (g *StatsGateway) FeedAmount(ctx context.Context) (int, error) {
	resp, err := g.client.GetFeedAmount(ctx, connect.NewRequest(&datahubv1.GetFeedAmountRequest{}))
	if err != nil {
		return 0, fmt.Errorf("get feed amount: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

func (g *StatsGateway) TotalArticlesCount(ctx context.Context) (int, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return 0, err
	}

	resp, err := g.client.GetTotalArticlesCount(ctx, connect.NewRequest(&datahubv1.GetTotalArticlesCountRequest{
		UserId: userID,
	}))
	if err != nil {
		return 0, fmt.Errorf("get total articles count: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

func (g *StatsGateway) SummarizedArticlesCount(ctx context.Context) (int, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return 0, err
	}

	resp, err := g.client.GetSummarizedArticlesCount(ctx, connect.NewRequest(&datahubv1.GetSummarizedArticlesCountRequest{
		UserId: userID,
	}))
	if err != nil {
		return 0, fmt.Errorf("get summarized articles count: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

func (g *StatsGateway) UnsummarizedArticlesCount(ctx context.Context) (int, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return 0, err
	}

	resp, err := g.client.GetUnsummarizedArticlesCount(ctx, connect.NewRequest(&datahubv1.GetUnsummarizedArticlesCountRequest{
		UserId: userID,
	}))
	if err != nil {
		return 0, fmt.Errorf("get unsummarized articles count: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

// TodayUnreadArticlesCount counts feeds newer than `since` the user has not
// read. `since` comes from the caller because "today" is a wall-clock question
// the provider has no timezone for.
func (g *StatsGateway) TodayUnreadArticlesCount(ctx context.Context, since time.Time) (int, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return 0, err
	}

	resp, err := g.client.GetTodayUnreadArticlesCount(ctx, connect.NewRequest(&datahubv1.GetTodayUnreadArticlesCountRequest{
		UserId: userID,
		Since:  timeToProto(since),
	}))
	if err != nil {
		return 0, fmt.Errorf("get today unread articles count: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

// TrendStats returns the admin dashboard series.
//
// The window arrives here as the string the HTTP query carried and is
// translated to the wire enum before the call, so an unsupported value is
// refused without a round trip — and refused with the message the driver's
// parseWindow produced, because that message is what the endpoint has always
// answered.
//
// The window is not echoed back over the wire. It is the caller's own input,
// and a provider that restated it would be a second place for the two to
// disagree; the adapter above puts it back on the port's response.
func (g *StatsGateway) TrendStats(ctx context.Context, window string) (*TrendSeries, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return nil, err
	}

	wire, err := trendWindowToProto(window)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.GetTrendStats(ctx, connect.NewRequest(&datahubv1.GetTrendStatsRequest{
		UserId: userID,
		Window: wire,
	}))
	if err != nil {
		return nil, fmt.Errorf("get trend stats for window %s: %w", window, err)
	}

	points := make([]TrendPoint, 0, len(resp.Msg.GetPoints()))
	for _, p := range resp.Msg.GetPoints() {
		if p == nil {
			continue
		}
		points = append(points, TrendPoint{
			Timestamp:    timeFromProto(p.GetBucket()),
			Articles:     int(p.GetArticles()),
			Summarized:   int(p.GetSummarized()),
			FeedActivity: int(p.GetFeedActivity()),
		})
	}

	return &TrendSeries{
		Points:      points,
		Granularity: trendGranularityFromProto(resp.Msg.GetGranularity()),
	}, nil
}

// UserFeedIDs returns the feeds the user has read state against.
func (g *StatsGateway) UserFeedIDs(ctx context.Context) ([]uuid.UUID, error) {
	userID, err := statsUserID(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.ListUserFeedIDs(ctx, connect.NewRequest(&datahubv1.ListUserFeedIDsRequest{
		UserId: userID,
	}))
	if err != nil {
		return nil, fmt.Errorf("list user feed ids: %w", err)
	}

	// nil rather than an empty slice for an empty answer, matching the driver:
	// the Morning Letter path checks len(), but a caller that distinguishes nil
	// from empty would otherwise see a change it never asked for.
	if len(resp.Msg.GetFeedIds()) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(resp.Msg.GetFeedIds()))
	for _, raw := range resp.Msg.GetFeedIds() {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("user feed id %q: %w", raw, parseErr)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// statsUserID resolves the signed-in tenant, refusing rather than defaulting.
//
// "authentication required" is the exact message the five drivers produced, and
// it is kept because the dashboard handlers above these gateways match on the
// error they surface. An unscoped fallback would be worse than an error here:
// these are COUNT queries whose only tenant predicate is this value, so a zero
// UUID answers 0 for everything and looks like an empty account rather than a
// failure.
func statsUserID(ctx context.Context) (string, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("authentication required: %w", err)
	}
	return user.UserID.String(), nil
}

// trendWindowToProto maps the query-string window onto the wire enum.
//
// The accepted set is the provider's — each value selects both a lower bound
// and the date_trunc unit the query groups by — so this is a translation, not a
// second validation. An unknown window never leaves the process.
func trendWindowToProto(window string) (datahubv1.TrendWindow, error) {
	switch window {
	case "4h":
		return datahubv1.TrendWindow_TREND_WINDOW_4H, nil
	case "24h":
		return datahubv1.TrendWindow_TREND_WINDOW_24H, nil
	case "3d":
		return datahubv1.TrendWindow_TREND_WINDOW_3D, nil
	case "7d":
		return datahubv1.TrendWindow_TREND_WINDOW_7D, nil
	default:
		return datahubv1.TrendWindow_TREND_WINDOW_UNSPECIFIED,
			fmt.Errorf("invalid window: unsupported window: %s", window)
	}
}

// trendGranularityFromProto returns the strings the chart renderer already
// switches on. An unspecified granularity becomes "" rather than a guess: the
// renderer treats the empty string as "no bucketing stated", where a wrong
// guess would label hourly points as daily.
func trendGranularityFromProto(g datahubv1.TrendGranularity) string {
	switch g {
	case datahubv1.TrendGranularity_TREND_GRANULARITY_HOURLY:
		return "hourly"
	case datahubv1.TrendGranularity_TREND_GRANULARITY_DAILY:
		return "daily"
	default:
		return ""
	}
}
