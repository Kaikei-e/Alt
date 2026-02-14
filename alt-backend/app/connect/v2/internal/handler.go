// Package internal provides the Connect-RPC handler for BackendInternalService.
package internal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	backendv1 "alt/gen/proto/services/backend/v1"
	"alt/gen/proto/services/backend/v1/backendv1connect"
	"alt/port/internal_article_port"
	"alt/port/internal_feed_port"
	"alt/port/internal_tag_port"
)

const maxLimit = 500

// Handler implements BackendInternalServiceHandler.
type Handler struct {
	// Phase 1 (search-indexer)
	listArticles        internal_article_port.ListArticlesWithTagsPort
	listArticlesForward internal_article_port.ListArticlesWithTagsForwardPort
	listDeleted         internal_article_port.ListDeletedArticlesPort
	getLatestTimestamp  internal_article_port.GetLatestArticleTimestampPort
	getArticleByID      internal_article_port.GetArticleByIDPort

	// Phase 2 (pre-processor)
	checkArticleExists internal_article_port.CheckArticleExistsPort
	createArticle      internal_article_port.CreateArticlePort
	saveArticleSummary internal_article_port.SaveArticleSummaryPort
	getArticleContent  internal_article_port.GetArticleContentPort
	getFeedID          internal_feed_port.GetFeedIDPort
	listFeedURLs       internal_feed_port.ListFeedURLsPort

	// Phase 3 (tag-generator)
	upsertArticleTags      internal_tag_port.UpsertArticleTagsPort
	batchUpsertArticleTags internal_tag_port.BatchUpsertArticleTagsPort
	listUntaggedArticles   internal_tag_port.ListUntaggedArticlesPort

	logger *slog.Logger
}

var _ backendv1connect.BackendInternalServiceHandler = (*Handler)(nil)

// NewHandler creates a new BackendInternalService handler.
func NewHandler(
	listArticles internal_article_port.ListArticlesWithTagsPort,
	listArticlesForward internal_article_port.ListArticlesWithTagsForwardPort,
	listDeleted internal_article_port.ListDeletedArticlesPort,
	getLatestTimestamp internal_article_port.GetLatestArticleTimestampPort,
	getArticleByID internal_article_port.GetArticleByIDPort,
	logger *slog.Logger,
	opts ...HandlerOption,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		listArticles:        listArticles,
		listArticlesForward: listArticlesForward,
		listDeleted:         listDeleted,
		getLatestTimestamp:  getLatestTimestamp,
		getArticleByID:      getArticleByID,
		logger:              logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandlerOption configures optional ports on the Handler.
type HandlerOption func(*Handler)

// WithPhase2Ports configures ports for Phase 2 (pre-processor) RPCs.
func WithPhase2Ports(
	checkExists internal_article_port.CheckArticleExistsPort,
	createArticle internal_article_port.CreateArticlePort,
	saveSummary internal_article_port.SaveArticleSummaryPort,
	getContent internal_article_port.GetArticleContentPort,
	getFeedID internal_feed_port.GetFeedIDPort,
	listFeedURLs internal_feed_port.ListFeedURLsPort,
) HandlerOption {
	return func(h *Handler) {
		h.checkArticleExists = checkExists
		h.createArticle = createArticle
		h.saveArticleSummary = saveSummary
		h.getArticleContent = getContent
		h.getFeedID = getFeedID
		h.listFeedURLs = listFeedURLs
	}
}

// WithPhase3Ports configures ports for Phase 3 (tag-generator) RPCs.
func WithPhase3Ports(
	upsertTags internal_tag_port.UpsertArticleTagsPort,
	batchUpsertTags internal_tag_port.BatchUpsertArticleTagsPort,
	listUntagged internal_tag_port.ListUntaggedArticlesPort,
) HandlerOption {
	return func(h *Handler) {
		h.upsertArticleTags = upsertTags
		h.batchUpsertArticleTags = batchUpsertTags
		h.listUntaggedArticles = listUntagged
	}
}

func (h *Handler) ListArticlesWithTags(ctx context.Context, req *connect.Request[backendv1.ListArticlesWithTagsRequest]) (*connect.Response[backendv1.ListArticlesWithTagsResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.listArticles.ListArticlesWithTags(ctx, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListArticlesWithTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles"))
	}

	resp := &backendv1.ListArticlesWithTagsResponse{
		Articles: toProtoArticles(articles),
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) ListArticlesWithTagsForward(ctx context.Context, req *connect.Request[backendv1.ListArticlesWithTagsForwardRequest]) (*connect.Response[backendv1.ListArticlesWithTagsForwardResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	incrementalMark := req.Msg.IncrementalMark.AsTime()

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.listArticlesForward.ListArticlesWithTagsForward(ctx, &incrementalMark, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListArticlesWithTagsForward failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles forward"))
	}

	resp := &backendv1.ListArticlesWithTagsForwardResponse{
		Articles: toProtoArticles(articles),
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) ListDeletedArticles(ctx context.Context, req *connect.Request[backendv1.ListDeletedArticlesRequest]) (*connect.Response[backendv1.ListDeletedArticlesResponse], error) {
	limit := clampLimit(int(req.Msg.Limit))

	var lastDeletedAt *time.Time
	if req.Msg.LastDeletedAt != nil {
		t := req.Msg.LastDeletedAt.AsTime()
		lastDeletedAt = &t
	}

	deletedArticles, nextDeletedAt, err := h.listDeleted.ListDeletedArticles(ctx, lastDeletedAt, limit)
	if err != nil {
		h.logger.Error("ListDeletedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list deleted articles"))
	}

	protoArticles := make([]*backendv1.DeletedArticle, len(deletedArticles))
	for i, da := range deletedArticles {
		protoArticles[i] = &backendv1.DeletedArticle{
			Id:        da.ID,
			DeletedAt: timestamppb.New(da.DeletedAt),
		}
	}

	resp := &backendv1.ListDeletedArticlesResponse{
		Articles: protoArticles,
	}
	if nextDeletedAt != nil {
		resp.NextDeletedAt = timestamppb.New(*nextDeletedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) GetLatestArticleTimestamp(ctx context.Context, _ *connect.Request[backendv1.GetLatestArticleTimestampRequest]) (*connect.Response[backendv1.GetLatestArticleTimestampResponse], error) {
	ts, err := h.getLatestTimestamp.GetLatestArticleTimestamp(ctx)
	if err != nil {
		h.logger.Error("GetLatestArticleTimestamp failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get latest timestamp"))
	}

	resp := &backendv1.GetLatestArticleTimestampResponse{}
	if ts != nil {
		resp.LatestCreatedAt = timestamppb.New(*ts)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) GetArticleByID(ctx context.Context, req *connect.Request[backendv1.GetArticleByIDRequest]) (*connect.Response[backendv1.GetArticleByIDResponse], error) {
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	article, err := h.getArticleByID.GetArticleByID(ctx, req.Msg.ArticleId)
	if err != nil {
		h.logger.Error("GetArticleByID failed", "article_id", req.Msg.ArticleId, "error", err)
		return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
	}

	return connect.NewResponse(&backendv1.GetArticleByIDResponse{
		Article: toProtoArticle(article),
	}), nil
}

// ── Phase 2: Article write operations (pre-processor) ──

func (h *Handler) CheckArticleExists(ctx context.Context, req *connect.Request[backendv1.CheckArticleExistsRequest]) (*connect.Response[backendv1.CheckArticleExistsResponse], error) {
	if h.checkArticleExists == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	exists, articleID, err := h.checkArticleExists.CheckArticleExists(ctx, req.Msg.Url, req.Msg.FeedId)
	if err != nil {
		h.logger.Error("CheckArticleExists failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check article existence"))
	}

	return connect.NewResponse(&backendv1.CheckArticleExistsResponse{
		Exists:    exists,
		ArticleId: articleID,
	}), nil
}

func (h *Handler) CreateArticle(ctx context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
	if h.createArticle == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.FeedId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_id is required"))
	}

	var publishedAt time.Time
	if req.Msg.PublishedAt != nil {
		publishedAt = req.Msg.PublishedAt.AsTime()
	}

	articleID, err := h.createArticle.CreateArticle(ctx, internal_article_port.CreateArticleParams{
		Title:       req.Msg.Title,
		URL:         req.Msg.Url,
		Content:     req.Msg.Content,
		FeedID:      req.Msg.FeedId,
		UserID:      req.Msg.UserId,
		PublishedAt: publishedAt,
	})
	if err != nil {
		h.logger.Error("CreateArticle failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create article"))
	}

	return connect.NewResponse(&backendv1.CreateArticleResponse{
		ArticleId: articleID,
	}), nil
}

func (h *Handler) SaveArticleSummary(ctx context.Context, req *connect.Request[backendv1.SaveArticleSummaryRequest]) (*connect.Response[backendv1.SaveArticleSummaryResponse], error) {
	if h.saveArticleSummary == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}
	if req.Msg.Summary == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("summary is required"))
	}

	err := h.saveArticleSummary.SaveArticleSummary(ctx, req.Msg.ArticleId, req.Msg.Summary, req.Msg.Language)
	if err != nil {
		h.logger.Error("SaveArticleSummary failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save article summary"))
	}

	return connect.NewResponse(&backendv1.SaveArticleSummaryResponse{
		Success: true,
	}), nil
}

func (h *Handler) GetArticleContent(ctx context.Context, req *connect.Request[backendv1.GetArticleContentRequest]) (*connect.Response[backendv1.GetArticleContentResponse], error) {
	if h.getArticleContent == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	content, err := h.getArticleContent.GetArticleContent(ctx, req.Msg.ArticleId)
	if err != nil {
		h.logger.Error("GetArticleContent failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article content"))
	}
	if content == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
	}

	return connect.NewResponse(&backendv1.GetArticleContentResponse{
		ArticleId: content.ID,
		Title:     content.Title,
		Content:   content.Content,
		Url:       content.URL,
	}), nil
}

func (h *Handler) GetFeedID(ctx context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
	if h.getFeedID == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	feedID, err := h.getFeedID.GetFeedID(ctx, req.Msg.FeedUrl)
	if err != nil {
		h.logger.Error("GetFeedID failed", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
	}

	return connect.NewResponse(&backendv1.GetFeedIDResponse{
		FeedId: feedID,
	}), nil
}

func (h *Handler) ListFeedURLs(ctx context.Context, req *connect.Request[backendv1.ListFeedURLsRequest]) (*connect.Response[backendv1.ListFeedURLsResponse], error) {
	if h.listFeedURLs == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))
	feeds, nextCursor, hasMore, err := h.listFeedURLs.ListFeedURLs(ctx, req.Msg.Cursor, limit)
	if err != nil {
		h.logger.Error("ListFeedURLs failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list feed URLs"))
	}

	protoFeeds := make([]*backendv1.FeedURL, len(feeds))
	for i, f := range feeds {
		protoFeeds[i] = &backendv1.FeedURL{
			FeedId: f.FeedID,
			Url:    f.URL,
		}
	}

	return connect.NewResponse(&backendv1.ListFeedURLsResponse{
		Feeds:      protoFeeds,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

// ── Phase 3: Tag operations (tag-generator) ──

func (h *Handler) UpsertArticleTags(ctx context.Context, req *connect.Request[backendv1.UpsertArticleTagsRequest]) (*connect.Response[backendv1.UpsertArticleTagsResponse], error) {
	if h.upsertArticleTags == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	tags := make([]internal_tag_port.TagItem, len(req.Msg.Tags))
	for i, t := range req.Msg.Tags {
		tags[i] = internal_tag_port.TagItem{Name: t.Name, Confidence: t.Confidence}
	}

	count, err := h.upsertArticleTags.UpsertArticleTags(ctx, req.Msg.ArticleId, req.Msg.FeedId, tags)
	if err != nil {
		h.logger.Error("UpsertArticleTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to upsert article tags"))
	}

	return connect.NewResponse(&backendv1.UpsertArticleTagsResponse{
		Success:       true,
		UpsertedCount: count,
	}), nil
}

func (h *Handler) BatchUpsertArticleTags(ctx context.Context, req *connect.Request[backendv1.BatchUpsertArticleTagsRequest]) (*connect.Response[backendv1.BatchUpsertArticleTagsResponse], error) {
	if h.batchUpsertArticleTags == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	items := make([]internal_tag_port.BatchUpsertItem, len(req.Msg.Items))
	for i, item := range req.Msg.Items {
		tags := make([]internal_tag_port.TagItem, len(item.Tags))
		for j, t := range item.Tags {
			tags[j] = internal_tag_port.TagItem{Name: t.Name, Confidence: t.Confidence}
		}
		items[i] = internal_tag_port.BatchUpsertItem{
			ArticleID: item.ArticleId,
			FeedID:    item.FeedId,
			Tags:      tags,
		}
	}

	total, err := h.batchUpsertArticleTags.BatchUpsertArticleTags(ctx, items)
	if err != nil {
		h.logger.Error("BatchUpsertArticleTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to batch upsert article tags"))
	}

	return connect.NewResponse(&backendv1.BatchUpsertArticleTagsResponse{
		Success:       true,
		TotalUpserted: total,
	}), nil
}

func (h *Handler) ListUntaggedArticles(ctx context.Context, req *connect.Request[backendv1.ListUntaggedArticlesRequest]) (*connect.Response[backendv1.ListUntaggedArticlesResponse], error) {
	if h.listUntaggedArticles == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))

	articles, totalCount, err := h.listUntaggedArticles.ListUntaggedArticles(ctx, limit, int(req.Msg.Offset))
	if err != nil {
		h.logger.Error("ListUntaggedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list untagged articles"))
	}

	protoArticles := make([]*backendv1.ArticleWithTags, len(articles))
	for i, a := range articles {
		protoArticles[i] = &backendv1.ArticleWithTags{
			Id:      a.ID,
			Title:   a.Title,
			Content: a.Content,
			UserId:  a.UserID,
		}
	}

	return connect.NewResponse(&backendv1.ListUntaggedArticlesResponse{
		Articles:   protoArticles,
		TotalCount: totalCount,
	}), nil
}

// ── Helpers ──

func toProtoArticles(articles []*internal_article_port.ArticleWithTags) []*backendv1.ArticleWithTags {
	result := make([]*backendv1.ArticleWithTags, len(articles))
	for i, a := range articles {
		result[i] = toProtoArticle(a)
	}
	return result
}

func toProtoArticle(a *internal_article_port.ArticleWithTags) *backendv1.ArticleWithTags {
	return &backendv1.ArticleWithTags{
		Id:        a.ID,
		Title:     a.Title,
		Content:   a.Content,
		Tags:      a.Tags,
		CreatedAt: timestamppb.New(a.CreatedAt),
		UserId:    a.UserID,
	}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		limit = 200
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}
