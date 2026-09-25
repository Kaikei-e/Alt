package articles

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"alt/connect/errorhandler"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/utils/safeconv"
)

// FetchArticlesByTag fetches articles by tag (ID or name).
// Replaces GET /v1/articles/by-tag
// ADR-169: tag_name で横断検索、tag_id は後方互換性
func (h *Handler) FetchArticlesByTag(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticlesByTagRequest],
) (*connect.Response[articlesv2.FetchArticlesByTagResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	limit := clampPageLimit(req.Msg.Limit, 20, maxArticlesPageSize)

	var cursor *time.Time
	if req.Msg.Cursor != nil && *req.Msg.Cursor != "" {
		parsed, err := time.Parse(time.RFC3339, *req.Msg.Cursor)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid cursor format, expected RFC3339: %w", err))
		}
		cursor = &parsed
	}

	var articles []*domain.TagTrailArticle
	var err error

	if req.Msg.TagName != nil && *req.Msg.TagName != "" {
		articles, err = h.deps.FetchArticlesByTag.ExecuteByTagName(ctx, *req.Msg.TagName, cursor, limit+1)
	} else if req.Msg.TagId != nil && *req.Msg.TagId != "" {
		articles, err = h.deps.FetchArticlesByTag.Execute(ctx, *req.Msg.TagId, cursor, limit+1)
	} else {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("either tag_id or tag_name is required"))
	}

	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticlesByTag")
	}

	hasMore := len(articles) > limit
	if hasMore {
		articles = articles[:limit]
	}

	protoArticles := convertTagTrailArticlesToProto(articles)

	// Derive next cursor, with sub-second precision: it comes back as the
	// right-hand side of a strict `created_at < $1`, and created_at is
	// microsecond precision.
	var nextCursor *string
	if len(articles) > 0 {
		nextCursor = deriveRFC3339NanoCursor(articles[len(articles)-1].PublishedAt, hasMore)
	}

	return connect.NewResponse(&articlesv2.FetchArticlesByTagResponse{
		Articles:   protoArticles,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// FetchArticleTags fetches tags for an article.
// Replaces GET /v1/articles/:id/tags
func (h *Handler) FetchArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleTagsRequest],
) (*connect.Response[articlesv2.FetchArticleTagsResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	tags, err := h.deps.FetchArticleTags.Execute(ctx, articleID)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleTags")
	}

	protoTags := convertTagsToProto(tags)

	return connect.NewResponse(&articlesv2.FetchArticleTagsResponse{
		ArticleId: articleID,
		Tags:      protoTags,
	}), nil
}

// StreamArticleTags streams real-time tag updates for an article.
// Returns cached tags immediately if available, otherwise triggers on-the-fly generation via mq-hub.
// ADR-168: On-the-fly tag generation for Tag Trail initial feed card.
func (h *Handler) StreamArticleTags(
	ctx context.Context,
	req *connect.Request[articlesv2.StreamArticleTagsRequest],
	stream *connect.ServerStream[articlesv2.StreamArticleTagsResponse],
) error {
	if _, err := requireUser(ctx); err != nil {
		return err
	}

	articleID := req.Msg.ArticleId
	if articleID == "" {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("article_id is required"))
	}

	h.logger.InfoContext(ctx, "starting article tags stream", "articleID", articleID)

	if h.deps.StreamArticleTags == nil {
		h.logger.WarnContext(ctx, "StreamArticleTagsUsecase not available, returning empty tags", "articleID", articleID)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation not available"),
		})
	}

	result, err := h.deps.StreamArticleTags.Execute(ctx, articleID)
	if err != nil {
		h.logger.WarnContext(ctx, "tag resolution failed", "articleID", articleID, "error", err)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("Tag generation temporarily unavailable"),
		})
	}

	if len(result.Tags) == 0 {
		h.logger.InfoContext(ctx, "no tags found or generated", "articleID", articleID)
		return stream.Send(&articlesv2.StreamArticleTagsResponse{
			ArticleId: articleID,
			Tags:      []*articlesv2.ArticleTagItem{},
			EventType: articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED,
			Message:   stringPtr("No tags generated"),
		})
	}

	eventType := articlesv2.StreamArticleTagsResponse_EVENT_TYPE_COMPLETED
	if result.IsCached {
		eventType = articlesv2.StreamArticleTagsResponse_EVENT_TYPE_CACHED
	}

	h.logger.InfoContext(ctx, "returning tags", "articleID", articleID, "tagCount", len(result.Tags), "cached", result.IsCached)
	return stream.Send(&articlesv2.StreamArticleTagsResponse{
		ArticleId: articleID,
		Tags:      convertTagsToProto(result.Tags),
		EventType: eventType,
	})
}

// FetchTagCloud fetches tag cloud data for Tag Verse visualization.
// Replaces GET /v1/articles/tag-cloud
func (h *Handler) FetchTagCloud(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchTagCloudRequest],
) (*connect.Response[articlesv2.FetchTagCloudResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	limit := clampPageLimit(req.Msg.Limit, 300, 500)

	items, err := h.deps.FetchTagCloud.Execute(ctx, limit)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchTagCloud")
	}

	protoItems := convertTagCloudItemsToProto(items)

	return connect.NewResponse(&articlesv2.FetchTagCloudResponse{
		Tags:      protoItems,
		TotalTags: safeconv.Int32(len(protoItems)),
	}), nil
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string {
	return &s
}
