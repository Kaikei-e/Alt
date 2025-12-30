// Package feeds implements the FeedService Connect-RPC handlers.
package feeds

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"

	"alt/connect/v2/middleware"
	"alt/di"
)

// Handler implements the FeedService Connect-RPC service.
type Handler struct {
	container *di.ApplicationComponents
	logger    *slog.Logger
}

// NewHandler creates a new Feed service handler.
func NewHandler(container *di.ApplicationComponents, logger *slog.Logger) *Handler {
	return &Handler{
		container: container,
		logger:    logger,
	}
}

// Verify interface implementation at compile time.
var _ feedsv2connect.FeedServiceHandler = (*Handler)(nil)

// GetFeedStats returns basic feed statistics (feed count, summarized count).
func (h *Handler) GetFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFeedStatsRequest],
) (*connect.Response[feedsv2.GetFeedStatsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to get feed count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	summarizedCount, err := h.container.SummarizedArticlesCountUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to get summarized count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&feedsv2.GetFeedStatsResponse{
		FeedAmount:           int64(feedCount),
		SummarizedFeedAmount: int64(summarizedCount),
	}), nil
}

// GetDetailedFeedStats returns detailed feed statistics.
func (h *Handler) GetDetailedFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.GetDetailedFeedStatsRequest],
) (*connect.Response[feedsv2.GetDetailedFeedStatsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to get feed count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	articleCount, err := h.container.TotalArticlesCountUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to get article count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	unsummarizedCount, err := h.container.UnsummarizedArticlesCountUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to get unsummarized count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&feedsv2.GetDetailedFeedStatsResponse{
		FeedAmount:             int64(feedCount),
		ArticleAmount:          int64(articleCount),
		UnsummarizedFeedAmount: int64(unsummarizedCount),
	}), nil
}

// GetUnreadCount returns the count of unread articles for today.
func (h *Handler) GetUnreadCount(
	ctx context.Context,
	req *connect.Request[feedsv2.GetUnreadCountRequest],
) (*connect.Response[feedsv2.GetUnreadCountResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Default to start of today (00:00:00 UTC)
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	count, err := h.container.TodayUnreadArticlesCountUsecase.Execute(ctx, since)
	if err != nil {
		h.logger.Error("failed to get unread count", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&feedsv2.GetUnreadCountResponse{
		Count: int64(count),
	}), nil
}
