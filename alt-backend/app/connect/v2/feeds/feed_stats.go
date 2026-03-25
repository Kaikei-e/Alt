package feeds

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
)

// GetFeedStats returns basic feed statistics (feed count, summarized count).
func (h *Handler) GetFeedStats(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFeedStatsRequest],
) (*connect.Response[feedsv2.GetFeedStatsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedCount, err := h.deps.FeedAmount.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFeedStats.FeedAmount")
	}

	summarizedCount, err := h.deps.SummarizedCount.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFeedStats.SummarizedCount")
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
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedCount, err := h.deps.FeedAmount.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.FeedAmount")
	}

	articleCount, err := h.deps.TotalCount.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.ArticleCount")
	}

	unsummarizedCount, err := h.deps.UnsummarizedCount.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetDetailedFeedStats.UnsummarizedCount")
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
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Default to start of today (00:00:00 UTC)
	now := time.Now().UTC()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	count, err := h.deps.TodayUnreadCount.Execute(ctx, since)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetUnreadCount")
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
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Get update intervals from config
	updateInterval := h.cfg.Server.SSEInterval
	if updateInterval == 0 {
		updateInterval = 5 * time.Second
	}
	heartbeatInterval := 10 * time.Second

	h.logger.InfoContext(ctx, "starting feed stats stream",
		"update_interval", updateInterval,
		"heartbeat_interval", heartbeatInterval)

	// Create tickers
	updateTicker := time.NewTicker(updateInterval)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Send initial data immediately
	if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
		return err
	}

	// Stream loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled
			h.logger.InfoContext(ctx, "feed stats stream cancelled", "reason", ctx.Err())
			return nil

		case <-updateTicker.C:
			// Send periodic data update
			if err := h.sendStatsUpdate(ctx, stream, false); err != nil {
				return err
			}

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive
			if err := h.sendStatsUpdate(ctx, stream, true); err != nil {
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
		feedCount, err := h.deps.FeedAmount.Execute(ctx)
		if err != nil {
			return fmt.Errorf("get feed count: %w", err)
		}

		unsummarized, err := h.deps.UnsummarizedCount.Execute(ctx)
		if err != nil {
			return fmt.Errorf("get unsummarized count: %w", err)
		}

		totalArticles, err := h.deps.TotalCount.Execute(ctx)
		if err != nil {
			return fmt.Errorf("get total articles: %w", err)
		}

		resp.FeedAmount = int64(feedCount)
		resp.UnsummarizedFeedAmount = int64(unsummarized)
		resp.TotalArticles = int64(totalArticles)
	}

	return stream.Send(resp)
}
