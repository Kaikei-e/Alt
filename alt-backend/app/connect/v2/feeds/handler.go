// Package feeds implements the FeedService Connect-RPC handlers.
package feeds

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/di"
)

// Handler implements the FeedService Connect-RPC service.
type Handler struct {
	container *di.ApplicationComponents
	logger    *slog.Logger
	cfg       *config.Config
}

// NewHandler creates a new Feed service handler.
func NewHandler(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		container: container,
		logger:    logger,
		cfg:       cfg,
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

// StreamFeedStats streams real-time feed statistics updates.
// Replaces the SSE endpoint /v1/sse/feeds/stats with Connect-RPC Server Streaming.
func (h *Handler) StreamFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamFeedStatsRequest],
	stream *connect.ServerStream[feedsv2.StreamFeedStatsResponse],
) error {
	// Authentication check
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Get update intervals from config
	updateInterval := h.cfg.Server.SSEInterval
	if updateInterval == 0 {
		updateInterval = 5 * time.Second
	}
	heartbeatInterval := 10 * time.Second

	h.logger.Info("starting feed stats stream",
		"update_interval", updateInterval,
		"heartbeat_interval", heartbeatInterval)

	// Create tickers
	updateTicker := time.NewTicker(updateInterval)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Send initial data immediately
	if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
		h.logger.Error("failed to send initial stats", "error", err)
		return err
	}

	// Stream loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled
			h.logger.Info("feed stats stream cancelled", "reason", ctx.Err())
			return nil

		case <-updateTicker.C:
			// Send periodic data update
			if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
				h.logger.Error("failed to send stats update", "error", err)
				return err
			}

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			if err := h.sendStatsUpdate(ctx, stream, true); err != nil {
				h.logger.Error("failed to send heartbeat", "error", err)
				return err
			}
		}
	}
}

// sendStatsUpdate sends a stats update or heartbeat message to the stream.
func (h *Handler) sendStatsUpdate(
	ctx context.Context,
	stream *connect.ServerStream[feedsv2.StreamFeedStatsResponse],
	isHeartbeat bool,
) error {
	resp := &feedsv2.StreamFeedStatsResponse{
		Metadata: &feedsv2.ResponseMetadata{
			Timestamp:   time.Now().Unix(),
			IsHeartbeat: isHeartbeat,
		},
	}

	if !isHeartbeat {
		// Fetch actual stats from usecases
		feedCount, err := h.container.FeedAmountUsecase.Execute(ctx)
		if err != nil {
			h.logger.Error("failed to get feed count", "error", err)
			return err
		}

		unsummarized, err := h.container.UnsummarizedArticlesCountUsecase.Execute(ctx)
		if err != nil {
			h.logger.Error("failed to get unsummarized count", "error", err)
			return err
		}

		totalArticles, err := h.container.TotalArticlesCountUsecase.Execute(ctx)
		if err != nil {
			h.logger.Error("failed to get total articles", "error", err)
			return err
		}

		resp.FeedAmount = int64(feedCount)
		resp.UnsummarizedFeedAmount = int64(unsummarized)
		resp.TotalArticles = int64(totalArticles)
	}

	return stream.Send(resp)
}
