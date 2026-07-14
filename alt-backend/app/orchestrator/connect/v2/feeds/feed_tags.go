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

const (
	defaultFeedTagsLimit = 20
	maxFeedTagsLimit     = 100
)

// GetFeedTags returns tags attached to a feed by ID.
// Replaces GET /v1/feeds/:id/tags used by Tag Trail.
func (h *Handler) GetFeedTags(
	ctx context.Context,
	req *connect.Request[feedsv2.GetFeedTagsRequest],
) (*connect.Response[feedsv2.GetFeedTagsResponse], error) {
	if _, err := middleware.GetUserContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_id is required"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = defaultFeedTagsLimit
	}
	if limit > maxFeedTagsLimit {
		limit = maxFeedTagsLimit
	}

	var cursor *time.Time
	if req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor: %w", err))
		}
		cursor = &parsed
	}

	if h.deps.AltDBRepository == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("feed tags repository not wired"))
	}

	tags, err := h.deps.AltDBRepository.FetchFeedTags(ctx, req.Msg.FeedId, cursor, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetFeedTags")
	}

	protoTags := make([]*feedsv2.FeedTag, 0, len(tags))
	for _, tag := range tags {
		protoTags = append(protoTags, &feedsv2.FeedTag{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&feedsv2.GetFeedTagsResponse{
		Tags: protoTags,
	}), nil
}
