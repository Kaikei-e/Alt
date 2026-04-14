// Package rss implements the RSSService Connect-RPC handlers.
package rss

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	rssv2 "alt/gen/proto/alt/rss/v2"
	"alt/gen/proto/alt/rss/v2/rssv2connect"

	"alt/config"
	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/di"
	"alt/domain"
	"alt/utils/url_validator"
)

// Handler implements the RSSService Connect-RPC service.
type Handler struct {
	container *di.ApplicationComponents
	logger    *slog.Logger
	cfg       *config.Config
}

// NewHandler creates a new RSS service handler.
func NewHandler(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		container: container,
		logger:    logger,
		cfg:       cfg,
	}
}

// Verify interface implementation at compile time.
var _ rssv2connect.RSSServiceHandler = (*Handler)(nil)

// RegisterRSSFeed registers a new RSS feed link.
// Replaces POST /v1/rss-feed-link/register
func (h *Handler) RegisterRSSFeed(
	ctx context.Context,
	req *connect.Request[rssv2.RegisterRSSFeedRequest],
) (*connect.Response[rssv2.RegisterRSSFeedResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL
	if strings.TrimSpace(req.Msg.Url) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.Url)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid URL format: %w", err))
	}

	// Check for allowed URLs (SSRF protection)
	if err := url_validator.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	if err := h.container.RegisterFeedsUsecase.Execute(ctx, req.Msg.Url); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RegisterRSSFeed")
	}

	return connect.NewResponse(&rssv2.RegisterRSSFeedResponse{
		Message: "RSS feed link registered",
	}), nil
}

// ListRSSFeedLinks returns all registered feed links.
// Replaces GET /v1/rss-feed-link/list
func (h *Handler) ListRSSFeedLinks(
	ctx context.Context,
	req *connect.Request[rssv2.ListRSSFeedLinksRequest],
) (*connect.Response[rssv2.ListRSSFeedLinksResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Call usecase (with health data)
	links, err := h.container.ListFeedLinksWithHealthUsecase.Execute(ctx)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "ListRSSFeedLinks")
	}

	// Convert to proto
	protoLinks := make([]*rssv2.RSSFeedLink, 0, len(links))
	for _, link := range links {
		protoLink := &rssv2.RSSFeedLink{
			Id:           link.ID.String(),
			Url:          link.URL,
			HealthStatus: string(link.GetHealthStatus()),
		}
		if link.Availability != nil {
			protoLink.ConsecutiveFailures = int32(link.Availability.ConsecutiveFailures)
			protoLink.IsActive = link.Availability.IsActive
			if link.Availability.LastFailureReason != nil {
				protoLink.LastFailureReason = *link.Availability.LastFailureReason
			}
		}
		protoLinks = append(protoLinks, protoLink)
	}

	return connect.NewResponse(&rssv2.ListRSSFeedLinksResponse{
		Links: protoLinks,
	}), nil
}

// DeleteRSSFeedLink removes a registered feed link.
// Replaces DELETE /v1/rss-feed-link/:id
func (h *Handler) DeleteRSSFeedLink(
	ctx context.Context,
	req *connect.Request[rssv2.DeleteRSSFeedLinkRequest],
) (*connect.Response[rssv2.DeleteRSSFeedLinkResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate ID
	if strings.TrimSpace(req.Msg.Id) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("id is required"))
	}

	linkID, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid feed link ID: %w", err))
	}

	// Call usecase
	if err := h.container.DeleteFeedLinkUsecase.Execute(ctx, userCtx.UserID, linkID); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "DeleteRSSFeedLink")
	}

	return connect.NewResponse(&rssv2.DeleteRSSFeedLinkResponse{
		Message: "Feed unsubscribed",
	}), nil
}

// RegisterFavoriteFeed marks a feed as favorite.
// Replaces POST /v1/feeds/register/favorite
func (h *Handler) RegisterFavoriteFeed(
	ctx context.Context,
	req *connect.Request[rssv2.RegisterFavoriteFeedRequest],
) (*connect.Response[rssv2.RegisterFavoriteFeedResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate URL (no SSRF check needed — this handler only does a DB lookup by URL,
	// it does not make external requests)
	if strings.TrimSpace(req.Msg.Url) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("url is required"))
	}

	// Call usecase
	if err := h.container.RegisterFavoriteFeedUsecase.Execute(ctx, req.Msg.Url); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RegisterFavoriteFeed")
	}

	return connect.NewResponse(&rssv2.RegisterFavoriteFeedResponse{
		Message: "favorite feed registered",
	}), nil
}

// RandomSubscription returns one random subscribed feed for Tag Trail discovery.
// Replaces GET /v1/rss-feed-link/random.
func (h *Handler) RandomSubscription(
	ctx context.Context,
	req *connect.Request[rssv2.RandomSubscriptionRequest],
) (*connect.Response[rssv2.RandomSubscriptionResponse], error) {
	_ = req
	if _, err := middleware.GetUserContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	if h.container == nil || h.container.FetchRandomSubscriptionUsecase == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("random subscription usecase not wired"))
	}
	feed, err := h.container.FetchRandomSubscriptionUsecase.Execute(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNoSubscriptions) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("no subscriptions available"))
		}
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RandomSubscription")
	}
	return connect.NewResponse(&rssv2.RandomSubscriptionResponse{
		Id:          feed.ID.String(),
		Title:       feed.Title,
		Description: feed.Description,
		Link:        feed.Link,
		PublishedAt: feed.UpdatedAt.Format(time.RFC3339),
	}), nil
}

// RemoveFavoriteFeed removes a feed from favorites.
func (h *Handler) RemoveFavoriteFeed(
	ctx context.Context,
	req *connect.Request[rssv2.RemoveFavoriteFeedRequest],
) (*connect.Response[rssv2.RemoveFavoriteFeedResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if strings.TrimSpace(req.Msg.Url) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("url is required"))
	}

	if err := h.container.RemoveFavoriteFeedUsecase.Execute(ctx, req.Msg.Url); err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "RemoveFavoriteFeed")
	}

	return connect.NewResponse(&rssv2.RemoveFavoriteFeedResponse{
		Message: "favorite feed removed",
	}), nil
}
