package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// §2.J Tag Trail paging
// ---------------------------------------------------------------------------

func (h *Handler) ListArticlesByTagID(ctx context.Context, req *connect.Request[datahubv1.ListArticlesByTagIDRequest]) (*connect.Response[datahubv1.ListArticlesByTagIDResponse], error) {
	tagID := req.Msg.GetTagId()
	if tagID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tag_id is required"))
	}

	articles, err := h.tagTrail.ArticlesByTagID(ctx, tagID, cursorFromProto(req.Msg.GetCursor()), clampTagLimit(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticlesByTagID failed", "error", err, "tag_id", tagID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles by tag id"))
	}
	return connect.NewResponse(&datahubv1.ListArticlesByTagIDResponse{
		Articles: tagTrailArticlesToProto(articles),
	}), nil
}

func (h *Handler) ListArticlesByTagName(ctx context.Context, req *connect.Request[datahubv1.ListArticlesByTagNameRequest]) (*connect.Response[datahubv1.ListArticlesByTagNameResponse], error) {
	tagName := req.Msg.GetTagName()
	if tagName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tag_name is required"))
	}

	articles, err := h.tagTrail.ArticlesByTagName(ctx, tagName, cursorFromProto(req.Msg.GetCursor()), clampTagLimit(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticlesByTagName failed", "error", err, "tag_name", tagName)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles by tag name"))
	}
	return connect.NewResponse(&datahubv1.ListArticlesByTagNameResponse{
		Articles: tagTrailArticlesToProto(articles),
	}), nil
}

// ---------------------------------------------------------------------------
// §2.J Tag reads
// ---------------------------------------------------------------------------

func (h *Handler) GetArticleTags(ctx context.Context, req *connect.Request[datahubv1.GetArticleTagsRequest]) (*connect.Response[datahubv1.GetArticleTagsResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	tags, err := h.tagRead.ArticleTags(ctx, req.Msg.GetArticleId())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleTags failed", "error", err, "article_id", req.Msg.GetArticleId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article tags"))
	}
	// An untagged article answers an empty list, not NotFound. The caller
	// treats emptiness as "ask mq-hub to generate some"; an error would make an
	// untagged article look like a database fault and suppress the generation.
	return connect.NewResponse(&datahubv1.GetArticleTagsResponse{Tags: feedTagsToProto(tags)}), nil
}

func (h *Handler) GetFeedTags(ctx context.Context, req *connect.Request[datahubv1.GetFeedTagsRequest]) (*connect.Response[datahubv1.GetFeedTagsResponse], error) {
	if req.Msg.GetFeedId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}
	limit := clampTagLimit(req.Msg.GetLimit())

	var cursor *time.Time
	if c := req.Msg.GetCursor(); c != nil {
		t := c.AsTime()
		cursor = &t
	}

	tags, err := h.tagRead.FeedTags(ctx, req.Msg.GetFeedId(), cursor, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedTags failed", "error", err, "feed_id", req.Msg.GetFeedId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed tags"))
	}
	return connect.NewResponse(&datahubv1.GetFeedTagsResponse{Tags: feedTagsToProto(tags)}), nil
}

func (h *Handler) GetTagCooccurrences(ctx context.Context, req *connect.Request[datahubv1.GetTagCooccurrencesRequest]) (*connect.Response[datahubv1.GetTagCooccurrencesResponse], error) {
	names := req.Msg.GetTagNames()
	if len(names) == 0 {
		return connect.NewResponse(&datahubv1.GetTagCooccurrencesResponse{}), nil
	}
	if len(names) > maxTagLimit {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("tag_names exceeds the %d entry limit", maxTagLimit))
	}

	items, err := h.tagRead.Cooccurrences(ctx, names)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTagCooccurrences failed", "error", err, "tag_count", len(names))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get tag cooccurrences"))
	}

	out := make([]*datahubv1.TagCooccurrence, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, &datahubv1.TagCooccurrence{
			TagNameA:    it.TagNameA,
			TagNameB:    it.TagNameB,
			SharedCount: safeconv.Int32(it.SharedCount),
		})
	}
	return connect.NewResponse(&datahubv1.GetTagCooccurrencesResponse{Cooccurrences: out}), nil
}

func (h *Handler) SearchTagsByPrefix(ctx context.Context, req *connect.Request[datahubv1.SearchTagsByPrefixRequest]) (*connect.Response[datahubv1.SearchTagsByPrefixResponse], error) {
	if req.Msg.GetPrefix() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("prefix is required"))
	}
	limit := clampTagLimit(req.Msg.GetLimit())

	hits, err := h.tagRead.SearchByPrefix(ctx, req.Msg.GetPrefix(), limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "SearchTagsByPrefix failed", "error", err, "prefix", req.Msg.GetPrefix())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search tags by prefix"))
	}

	out := make([]*datahubv1.TagPrefixHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, &datahubv1.TagPrefixHit{
			TagName:      hit.TagName,
			ArticleCount: safeconv.Int32(hit.ArticleCount),
		})
	}
	return connect.NewResponse(&datahubv1.SearchTagsByPrefixResponse{Hits: out}), nil
}

func (h *Handler) GetTagArticleCounts(ctx context.Context, req *connect.Request[datahubv1.GetTagArticleCountsRequest]) (*connect.Response[datahubv1.GetTagArticleCountsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	// `since` is required rather than defaulted. Without it the query counts
	// the user's entire history, which is a different and far more expensive
	// question than the one every caller asks; defaulting would answer it
	// silently.
	if req.Msg.GetSince() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("since is required"))
	}

	counts, err := h.tagRead.ArticleCounts(ctx, userID, req.Msg.GetSince().AsTime())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTagArticleCounts failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get tag article counts"))
	}

	out := make([]*datahubv1.TagArticleCount, 0, len(counts))
	for _, c := range counts {
		out = append(out, &datahubv1.TagArticleCount{
			TagName:      c.TagName,
			ArticleCount: safeconv.Int32(c.ArticleCount),
		})
	}
	return connect.NewResponse(&datahubv1.GetTagArticleCountsResponse{Counts: out}), nil
}

func (h *Handler) ListUntaggedArticles(ctx context.Context, req *connect.Request[datahubv1.ListUntaggedArticlesRequest]) (*connect.Response[datahubv1.ListUntaggedArticlesResponse], error) {
	if h.listUntaggedArticles == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, totalCount, err := h.listUntaggedArticles.ListUntaggedArticles(ctx, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListUntaggedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list untagged articles"))
	}

	protoArticles := make([]*datahubv1.ArticleWithTags, len(articles))
	for i, a := range articles {
		var feedID string
		if a.FeedID != nil {
			feedID = *a.FeedID
		}
		protoArticles[i] = &datahubv1.ArticleWithTags{
			Id:        a.ID,
			Title:     a.Title,
			Content:   a.Content,
			UserId:    a.UserID,
			FeedId:    feedID,
			CreatedAt: timestamppb.New(a.CreatedAt),
		}
	}

	resp := &datahubv1.ListUntaggedArticlesResponse{
		Articles:   protoArticles,
		TotalCount: totalCount,
		NextId:     nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

// BatchGetTagsByArticleIDs returns tags for a batch of article ids.
// Replaces tag-generator's /api/v1/tags/batch (ADR-000241 / ADR-000397)
// so recap-worker reads tags directly from the alt-backend data owner.
func (h *Handler) BatchGetTagsByArticleIDs(ctx context.Context, req *connect.Request[datahubv1.BatchGetTagsByArticleIDsRequest]) (*connect.Response[datahubv1.BatchGetTagsByArticleIDsResponse], error) {
	if h.batchGetTagsByArticleIDs == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	ids := req.Msg.GetArticleIds()
	if len(ids) == 0 {
		return connect.NewResponse(&datahubv1.BatchGetTagsByArticleIDsResponse{}), nil
	}
	if len(ids) > maxBatchGetTagsArticleIDs {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article_ids exceeds max batch size %d", maxBatchGetTagsArticleIDs))
	}

	grouped, err := h.batchGetTagsByArticleIDs.BatchGetTagsByArticleIDs(ctx, ids)
	if err != nil {
		h.logger.Error("BatchGetTagsByArticleIDs failed", "error", err, "article_count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to batch fetch article tags"))
	}

	items := make([]*datahubv1.ArticleTagsEntry, 0, len(grouped))
	for _, g := range grouped {
		tags := make([]*datahubv1.ArticleTagEntry, len(g.Tags))
		for i, t := range g.Tags {
			tags[i] = &datahubv1.ArticleTagEntry{
				TagName:    t.TagName,
				Confidence: t.Confidence,
				Source:     t.Source,
				UpdatedAt:  timestamppb.New(t.UpdatedAt),
			}
		}
		items = append(items, &datahubv1.ArticleTagsEntry{
			ArticleId: g.ArticleID,
			Tags:      tags,
		})
	}

	return connect.NewResponse(&datahubv1.BatchGetTagsByArticleIDsResponse{Items: items}), nil
}

// maxBatchGetTagsArticleIDs mirrors the caller-side guard previously
// enforced by tag-generator/app/auth_service.py. Kept as a server-side
// invariant so the driver never issues an unbounded ANY($1::uuid[]).
const maxBatchGetTagsArticleIDs = 1000

// FetchTagCloud returns tag cloud data for topic exploration.
func (h *Handler) FetchTagCloud(ctx context.Context, req *connect.Request[datahubv1.FetchTagCloudRequest]) (*connect.Response[datahubv1.FetchTagCloudResponse], error) {
	if h.fetchTagCloudPort == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("FetchTagCloud not configured"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 300
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	items, err := h.fetchTagCloudPort.Execute(ctx, limit)
	if err != nil {
		h.logger.Error("FetchTagCloud failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch tag cloud: %w", err))
	}

	tags := make([]*datahubv1.TagCloudItem, 0, len(items))
	for _, item := range items {
		tags = append(tags, &datahubv1.TagCloudItem{
			TagName:      item.TagName,
			ArticleCount: safeconv.Int32(item.ArticleCount),
		})
	}

	return connect.NewResponse(&datahubv1.FetchTagCloudResponse{
		Tags: tags,
	}), nil
}

// FetchArticlesByTag returns articles matching a tag name.
func (h *Handler) FetchArticlesByTag(ctx context.Context, req *connect.Request[datahubv1.FetchArticlesByTagRequest]) (*connect.Response[datahubv1.FetchArticlesByTagResponse], error) {
	if h.fetchArticlesByTagPort == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("FetchArticlesByTag not configured"))
	}

	tagName := req.Msg.TagName
	if tagName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tag_name is required"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	articles, err := h.fetchArticlesByTagPort.ExecuteByTagName(ctx, tagName, nil, limit)
	if err != nil {
		h.logger.Error("FetchArticlesByTag failed", "error", err, "tag", tagName)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch articles by tag: %w", err))
	}

	result := make([]*datahubv1.ArticleByTagItem, 0, len(articles))
	for _, a := range articles {
		result = append(result, &datahubv1.ArticleByTagItem{
			Id:          a.ID,
			Title:       a.Title,
			Url:         a.Link,
			PublishedAt: a.PublishedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&datahubv1.FetchArticlesByTagResponse{
		Articles: result,
	}), nil
}
