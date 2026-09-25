package datahubapi

import (
	"context"
	"errors"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// GetFeedAmount is the one count with no tenant, because it is the
// deployment's size rather than a user's.
func (h *Handler) GetFeedAmount(ctx context.Context, _ *connect.Request[datahubv1.GetFeedAmountRequest]) (*connect.Response[datahubv1.GetFeedAmountResponse], error) {
	count, err := h.stats.FeedAmount(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedAmount failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed amount"))
	}
	return connect.NewResponse(&datahubv1.GetFeedAmountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetTotalArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetTotalArticlesCountRequest]) (*connect.Response[datahubv1.GetTotalArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.TotalArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTotalArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get total articles count"))
	}
	return connect.NewResponse(&datahubv1.GetTotalArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetSummarizedArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetSummarizedArticlesCountRequest]) (*connect.Response[datahubv1.GetSummarizedArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.SummarizedArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetSummarizedArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get summarized articles count"))
	}
	return connect.NewResponse(&datahubv1.GetSummarizedArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetUnsummarizedArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetUnsummarizedArticlesCountRequest]) (*connect.Response[datahubv1.GetUnsummarizedArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.UnsummarizedArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetUnsummarizedArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get unsummarized articles count"))
	}
	return connect.NewResponse(&datahubv1.GetUnsummarizedArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

// GetTodayUnreadArticlesCount requires `since` rather than defaulting it.
//
// A server-side midnight would answer a different question convincingly: the
// provider does not know the reader's timezone, so "today" is only meaningful
// where the request came from.
func (h *Handler) GetTodayUnreadArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetTodayUnreadArticlesCountRequest]) (*connect.Response[datahubv1.GetTodayUnreadArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetSince() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("since is required"))
	}

	count, err := h.stats.TodayUnread(ctx, userID, req.Msg.GetSince().AsTime())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTodayUnreadArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get today unread articles count"))
	}
	return connect.NewResponse(&datahubv1.GetTodayUnreadArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetTrendStats(ctx context.Context, req *connect.Request[datahubv1.GetTrendStatsRequest]) (*connect.Response[datahubv1.GetTrendStatsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	window, err := trendWindowFromProto(req.Msg.GetWindow())
	if err != nil {
		return nil, err
	}

	series, err := h.stats.TrendStats(ctx, userID, window)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTrendStats failed", "error", err, "user_id", userID, "window", window)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get trend stats"))
	}

	points := make([]*datahubv1.TrendDataPoint, 0, len(series.Points))
	for _, p := range series.Points {
		points = append(points, &datahubv1.TrendDataPoint{
			Bucket:       timestamppb.New(p.Timestamp),
			Articles:     safeconv.Int32(p.Articles),
			Summarized:   safeconv.Int32(p.Summarized),
			FeedActivity: safeconv.Int32(p.FeedActivity),
		})
	}

	return connect.NewResponse(&datahubv1.GetTrendStatsResponse{
		Points:      points,
		Granularity: trendGranularityToProto(series.Granularity),
	}), nil
}

func (h *Handler) ListUserFeedIDs(ctx context.Context, req *connect.Request[datahubv1.ListUserFeedIDsRequest]) (*connect.Response[datahubv1.ListUserFeedIDsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	ids, err := h.stats.UserFeedIDs(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListUserFeedIDs failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list user feed ids"))
	}
	return connect.NewResponse(&datahubv1.ListUserFeedIDsResponse{FeedIds: uuidStrings(ids)}), nil
}

// trendWindowFromProto rejects the unspecified value instead of picking a
// default window.
//
// Defaulting would answer a question nobody asked and charge for it: the four
// windows differ by two orders of magnitude in rows scanned, and a caller that
// forgot the field would silently get whichever one this function preferred.
func trendWindowFromProto(w datahubv1.TrendWindow) (string, error) {
	switch w {
	case datahubv1.TrendWindow_TREND_WINDOW_4H:
		return "4h", nil
	case datahubv1.TrendWindow_TREND_WINDOW_24H:
		return "24h", nil
	case datahubv1.TrendWindow_TREND_WINDOW_3D:
		return "3d", nil
	case datahubv1.TrendWindow_TREND_WINDOW_7D:
		return "7d", nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("window is required"))
	}
}

func trendGranularityToProto(granularity string) datahubv1.TrendGranularity {
	switch granularity {
	case "hourly":
		return datahubv1.TrendGranularity_TREND_GRANULARITY_HOURLY
	case "daily":
		return datahubv1.TrendGranularity_TREND_GRANULARITY_DAILY
	default:
		return datahubv1.TrendGranularity_TREND_GRANULARITY_UNSPECIFIED
	}
}
