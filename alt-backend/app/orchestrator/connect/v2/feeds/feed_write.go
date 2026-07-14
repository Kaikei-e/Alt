package feeds

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
)

// MarkAsRead marks an article as read by its URL.
// Replaces POST /v1/feeds/read
func (h *Handler) MarkAsRead(
	ctx context.Context,
	req *connect.Request[feedsv2.MarkAsReadRequest],
) (*connect.Response[feedsv2.MarkAsReadResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate article_url
	if req.Msg.ArticleUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_url is required"))
	}

	// Parse URL
	articleURL, err := url.Parse(req.Msg.ArticleUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid article_url: %w", err))
	}

	// Execute usecase
	if err := h.deps.ArticlesReadingStatus.Execute(ctx, *articleURL); err != nil {
		// Map domain errors to appropriate HTTP status codes
		if errors.Is(err, domain.ErrFeedNotFound) {
			h.logger.InfoContext(ctx, "feed not found for mark as read",
				"article_url", req.Msg.ArticleUrl,
				"error", err)
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("feed not found: %s", req.Msg.ArticleUrl))
		}

		// All other errors are internal server errors
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "MarkAsRead")
	}

	h.logger.InfoContext(ctx, "feed marked as read", "article_url", req.Msg.ArticleUrl)

	return connect.NewResponse(&feedsv2.MarkAsReadResponse{
		Message: "Feed read status updated",
	}), nil
}

// Subscribe subscribes the current user to a feed source.
func (h *Handler) Subscribe(
	ctx context.Context,
	req *connect.Request[feedsv2.SubscribeRequest],
) (*connect.Response[feedsv2.SubscribeResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.FeedLinkId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_link_id is required"))
	}

	feedLinkID, err := uuid.Parse(req.Msg.FeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feed_link_id: %w", err))
	}

	if err := h.deps.Subscribe.Execute(ctx, userCtx.UserID, feedLinkID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "Subscribe")
	}

	return connect.NewResponse(&feedsv2.SubscribeResponse{
		Message: "Subscribed successfully",
	}), nil
}

// Unsubscribe unsubscribes the current user from a feed source.
func (h *Handler) Unsubscribe(
	ctx context.Context,
	req *connect.Request[feedsv2.UnsubscribeRequest],
) (*connect.Response[feedsv2.UnsubscribeResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.FeedLinkId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_link_id is required"))
	}

	feedLinkID, err := uuid.Parse(req.Msg.FeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feed_link_id: %w", err))
	}

	if err := h.deps.Unsubscribe.Execute(ctx, userCtx.UserID, feedLinkID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "Unsubscribe")
	}

	return connect.NewResponse(&feedsv2.UnsubscribeResponse{
		Message: "Unsubscribed successfully",
	}), nil
}
