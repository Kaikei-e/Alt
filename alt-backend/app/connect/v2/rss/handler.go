// Package rss implements the RSSService Connect-RPC handlers.
package rss

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	rssv2 "alt/gen/proto/alt/rss/v2"
	"alt/gen/proto/alt/rss/v2/rssv2connect"

	"alt/config"
	"alt/connect/v2/middleware"
	"alt/di"
	"alt/rest"
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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
	if err := rest.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	if err := h.container.RegisterFeedsUsecase.Execute(ctx, req.Msg.Url); err != nil {
		h.logger.Error("failed to register RSS feed", "error", err, "url", req.Msg.Url)
		return nil, connect.NewError(connect.CodeInternal, err)
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	// Call usecase
	links, err := h.container.ListFeedLinksUsecase.Execute(ctx)
	if err != nil {
		h.logger.Error("failed to list feed links", "error", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Convert to proto
	protoLinks := make([]*rssv2.RSSFeedLink, 0, len(links))
	for _, link := range links {
		protoLinks = append(protoLinks, &rssv2.RSSFeedLink{
			Id:  link.ID.String(),
			Url: link.URL,
		})
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
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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
	if err := h.container.DeleteFeedLinkUsecase.Execute(ctx, linkID); err != nil {
		h.logger.Error("failed to delete feed link", "error", err, "id", req.Msg.Id)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&rssv2.DeleteRSSFeedLinkResponse{
		Message: "Feed link deleted",
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
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
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
	if err := rest.IsAllowedURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	if err := h.container.RegisterFavoriteFeedUsecase.Execute(ctx, req.Msg.Url); err != nil {
		h.logger.Error("failed to register favorite feed", "error", err, "url", req.Msg.Url)
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&rssv2.RegisterFavoriteFeedResponse{
		Message: "favorite feed registered",
	}), nil
}
