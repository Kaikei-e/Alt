package feeds

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/utils/perf"

	"google.golang.org/protobuf/proto"
)

// parseExcludeFeedLinkIDs parses exclude feed link IDs from the request.
// Prefers the repeated field; falls back to the deprecated single field.
func parseExcludeFeedLinkIDs(repeatedIDs []string, singleID *string) ([]uuid.UUID, error) {
	if len(repeatedIDs) > 0 {
		if len(repeatedIDs) > 50 {
			return nil, fmt.Errorf("too many exclude IDs (max 50)")
		}
		ids := make([]uuid.UUID, 0, len(repeatedIDs))
		for _, s := range repeatedIDs {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("invalid exclude_feed_link_ids element: %w", err)
			}
			ids = append(ids, id)
		}
		return ids, nil
	}
	if singleID != nil && *singleID != "" {
		id, err := uuid.Parse(*singleID)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude_feed_link_id: %w", err)
		}
		return []uuid.UUID{id}, nil
	}
	return nil, nil
}

// =============================================================================
// Feed List RPCs (Phase 2)
// =============================================================================

// GetUnreadFeeds returns unread feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/cursor
func (h *Handler) GetUnreadFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetUnreadFeedsRequest],
) (*connect.Response[feedsv2.GetUnreadFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
		if req.Msg.View != nil && *req.Msg.View == "swipe" {
			limit = 1
		}
	}
	if limit > 100 {
		limit = 100
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Parse exclude feed link IDs (prefer repeated field, fallback to single)
	excludeFeedLinkIDs, err := parseExcludeFeedLinkIDs(req.Msg.ExcludeFeedLinkIds, req.Msg.ExcludeFeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Call usecase
	timer := perf.NewFeedReadTimer("GetUnreadFeeds")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	feeds, hasMore, err := h.deps.CachedFeedList.FetchUnreadFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetUnreadFeeds")
	}

	h.enrichWithProxyURLs(feeds)

	stopMarshal := timer.StartPhase(ctx, "marshal")
	respMsg := &feedsv2.GetUnreadFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}
	resp := connect.NewResponse(respMsg)
	stopMarshal()

	timer.SetRowCount(len(feeds))
	timer.SetPayloadBytes(int64(proto.Size(respMsg)))
	timer.Log(ctx)
	return resp, nil
}

// GetAllFeeds returns all feeds (read + unread) with cursor-based pagination.
func (h *Handler) GetAllFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetAllFeedsRequest],
) (*connect.Response[feedsv2.GetAllFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Parse exclude feed link IDs (prefer repeated field, fallback to single)
	excludeFeedLinkIDs, err := parseExcludeFeedLinkIDs(req.Msg.ExcludeFeedLinkIds, req.Msg.ExcludeFeedLinkId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Call usecase (all feeds, no read status filter)
	timer := perf.NewFeedReadTimer("GetAllFeeds")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	feeds, err := h.deps.CachedFeedList.FetchAllFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetAllFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	h.enrichWithProxyURLs(feeds)

	stopMarshal := timer.StartPhase(ctx, "marshal")
	respMsg := &feedsv2.GetAllFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	}
	resp := connect.NewResponse(respMsg)
	stopMarshal()

	timer.SetRowCount(len(feeds))
	timer.SetPayloadBytes(int64(proto.Size(respMsg)))
	timer.Log(ctx)
	return resp, nil
}

// GetReadFeeds returns read/viewed feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/viewed/cursor
func (h *Handler) GetReadFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetReadFeedsRequest],
) (*connect.Response[feedsv2.GetReadFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 32 // default for read feeds
	}
	if limit > 100 {
		limit = 100
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Call usecase
	timer := perf.NewFeedReadTimer("GetReadFeeds")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	feeds, err := h.deps.FetchReadFeedsCursor.Execute(ctx, cursor, limit)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetReadFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	h.enrichWithProxyURLs(feeds)

	stopMarshal := timer.StartPhase(ctx, "marshal")
	resp := connect.NewResponse(&feedsv2.GetReadFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	})
	stopMarshal()

	timer.SetRowCount(len(feeds))
	timer.Log(ctx)
	return resp, nil
}

// GetFavoriteFeeds returns favorite feeds with cursor-based pagination.
// Replaces GET /v1/feeds/fetch/favorites/cursor
func (h *Handler) GetFavoriteFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFavoriteFeedsRequest],
) (*connect.Response[feedsv2.GetFavoriteFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Parse and validate limit
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100
	}

	// Parse cursor if provided
	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	// Call usecase
	timer := perf.NewFeedReadTimer("GetFavoriteFeeds")

	stopUsecase := timer.StartPhase(ctx, "usecase")
	feeds, err := h.deps.FetchFavoriteFeedsCursor.Execute(ctx, cursor, limit)
	stopUsecase()
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFavoriteFeeds")
	}

	// Determine hasMore based on result count vs requested limit
	hasMore := len(feeds) >= limit

	h.enrichWithProxyURLs(feeds)

	stopMarshal := timer.StartPhase(ctx, "marshal")
	resp := connect.NewResponse(&feedsv2.GetFavoriteFeedsResponse{
		Data:       convertFeedsToProto(feeds),
		NextCursor: deriveNextCursor(feeds, hasMore),
		HasMore:    hasMore,
	})
	stopMarshal()

	timer.SetRowCount(len(feeds))
	timer.Log(ctx)
	return resp, nil
}

// =============================================================================
// Feed Search RPC (Phase 3)
// =============================================================================

// SearchFeeds searches for feeds by query with offset-based pagination.
// Replaces POST /v1/feeds/search
func (h *Handler) SearchFeeds(
	ctx context.Context,
	req *connect.Request[feedsv2.SearchFeedsRequest],
) (*connect.Response[feedsv2.SearchFeedsResponse], error) {
	_, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Validate query
	if req.Msg.Query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must not be empty"))
	}

	// Parse pagination params (offset-based)
	offset := 0
	if req.Msg.Cursor != nil {
		offset = int(*req.Msg.Cursor)
		if offset < 0 {
			offset = 0
		}
	}

	limit := 20
	if req.Msg.Limit != nil {
		limit = int(*req.Msg.Limit)
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
	}

	// Call usecase with pagination
	results, hasMore, err := h.deps.FeedSearch.ExecuteWithPagination(
		ctx, req.Msg.Query, offset, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "SearchFeeds")
	}

	// Compute next cursor
	var nextCursor *int32
	if hasMore {
		next := int32(offset + len(results))
		nextCursor = &next
	}

	return connect.NewResponse(&feedsv2.SearchFeedsResponse{
		Data:       convertFeedsToProto(results),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// =============================================================================
// Subscription RPCs (Read)
// =============================================================================

// ListSubscriptions returns all feed sources with subscription status for the current user.
func (h *Handler) ListSubscriptions(
	ctx context.Context,
	req *connect.Request[feedsv2.ListSubscriptionsRequest],
) (*connect.Response[feedsv2.ListSubscriptionsResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	sources, err := h.deps.ListSubscriptions.Execute(ctx, userCtx.UserID)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "ListSubscriptions")
	}

	protoSources := make([]*feedsv2.FeedSource, 0, len(sources))
	for _, s := range sources {
		protoSources = append(protoSources, &feedsv2.FeedSource{
			Id:           s.ID,
			Url:          s.URL,
			Title:        s.Title,
			IsSubscribed: s.IsSubscribed,
			CreatedAt:    s.CreatedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&feedsv2.ListSubscriptionsResponse{
		Sources: protoSources,
	}), nil
}
