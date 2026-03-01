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
	"alt/port/event_publisher_port"
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

	// Phase 4 (quality checker)
	deleteArticleSummary      internal_article_port.DeleteArticleSummaryPort
	checkArticleSummaryExists internal_article_port.CheckArticleSummaryExistsPort
	findArticlesWithSummaries internal_article_port.FindArticlesWithSummariesPort

	// Summarization (pre-processor polling)
	listUnsummarized internal_article_port.ListUnsummarizedArticlesPort
	hasUnsummarized  internal_article_port.HasUnsummarizedArticlesPort

	// Event publishing
	eventPublisher event_publisher_port.EventPublisherPort

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

// WithSummarizationPorts configures ports for summarization polling RPCs.
func WithSummarizationPorts(
	listUnsummarized internal_article_port.ListUnsummarizedArticlesPort,
	hasUnsummarized internal_article_port.HasUnsummarizedArticlesPort,
) HandlerOption {
	return func(h *Handler) {
		h.listUnsummarized = listUnsummarized
		h.hasUnsummarized = hasUnsummarized
	}
}

// WithEventPublisher configures the event publisher for domain events.
func WithEventPublisher(ep event_publisher_port.EventPublisherPort) HandlerOption {
	return func(h *Handler) {
		h.eventPublisher = ep
	}
}

// WithPhase4Ports configures ports for Phase 4 (quality checker) RPCs.
func WithPhase4Ports(
	deleteSummary internal_article_port.DeleteArticleSummaryPort,
	checkSummaryExists internal_article_port.CheckArticleSummaryExistsPort,
	findWithSummaries internal_article_port.FindArticlesWithSummariesPort,
) HandlerOption {
	return func(h *Handler) {
		h.deleteArticleSummary = deleteSummary
		h.checkArticleSummaryExists = checkSummaryExists
		h.findArticlesWithSummaries = findWithSummaries
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

	// Fire-and-forget: publish ArticleCreated event for downstream consumers
	if h.eventPublisher != nil && h.eventPublisher.IsEnabled() {
		if pubErr := h.eventPublisher.PublishArticleCreated(ctx, event_publisher_port.ArticleCreatedEvent{
			ArticleID:   articleID,
			UserID:      req.Msg.UserId,
			FeedID:      req.Msg.FeedId,
			Title:       req.Msg.Title,
			URL:         req.Msg.Url,
			Content:     req.Msg.Content,
			PublishedAt: publishedAt,
		}); pubErr != nil {
			h.logger.Warn("failed to publish ArticleCreated event (non-fatal)",
				"article_id", articleID, "error", pubErr)
		}
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
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
	}

	err := h.saveArticleSummary.SaveArticleSummary(ctx, req.Msg.ArticleId, req.Msg.UserId, req.Msg.Summary, req.Msg.Language)
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
		UserId:    content.UserID,
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
		var feedID string
		if a.FeedID != nil {
			feedID = *a.FeedID
		}
		protoArticles[i] = &backendv1.ArticleWithTags{
			Id:      a.ID,
			Title:   a.Title,
			Content: a.Content,
			UserId:  a.UserID,
			FeedId:  feedID,
		}
	}

	return connect.NewResponse(&backendv1.ListUntaggedArticlesResponse{
		Articles:   protoArticles,
		TotalCount: totalCount,
	}), nil
}

// ── Phase 4: Summary quality operations (quality checker) ──

func (h *Handler) DeleteArticleSummary(ctx context.Context, req *connect.Request[backendv1.DeleteArticleSummaryRequest]) (*connect.Response[backendv1.DeleteArticleSummaryResponse], error) {
	if h.deleteArticleSummary == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	err := h.deleteArticleSummary.DeleteArticleSummary(ctx, req.Msg.ArticleId)
	if err != nil {
		h.logger.Error("DeleteArticleSummary failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete article summary"))
	}

	return connect.NewResponse(&backendv1.DeleteArticleSummaryResponse{
		Success: true,
	}), nil
}

func (h *Handler) CheckArticleSummaryExists(ctx context.Context, req *connect.Request[backendv1.CheckArticleSummaryExistsRequest]) (*connect.Response[backendv1.CheckArticleSummaryExistsResponse], error) {
	if h.checkArticleSummaryExists == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	exists, summaryID, err := h.checkArticleSummaryExists.CheckArticleSummaryExists(ctx, req.Msg.ArticleId)
	if err != nil {
		h.logger.Error("CheckArticleSummaryExists failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check article summary existence"))
	}

	return connect.NewResponse(&backendv1.CheckArticleSummaryExistsResponse{
		Exists:    exists,
		SummaryId: summaryID,
	}), nil
}

func (h *Handler) FindArticlesWithSummaries(ctx context.Context, req *connect.Request[backendv1.FindArticlesWithSummariesRequest]) (*connect.Response[backendv1.FindArticlesWithSummariesResponse], error) {
	if h.findArticlesWithSummaries == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.findArticlesWithSummaries.FindArticlesWithSummaries(ctx, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("FindArticlesWithSummaries failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to find articles with summaries"))
	}

	protoArticles := make([]*backendv1.ArticleWithSummaryItem, len(articles))
	for i, a := range articles {
		protoArticles[i] = &backendv1.ArticleWithSummaryItem{
			ArticleId:       a.ArticleID,
			ArticleContent:  a.ArticleContent,
			ArticleUrl:      a.ArticleURL,
			SummaryId:       a.SummaryID,
			SummaryJapanese: a.SummaryJapanese,
			CreatedAt:       timestamppb.New(a.CreatedAt),
		}
	}

	resp := &backendv1.FindArticlesWithSummariesResponse{
		Articles: protoArticles,
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

// ── Summarization operations (pre-processor polling) ──

func (h *Handler) ListUnsummarizedArticles(ctx context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
	if h.listUnsummarized == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	limit := clampLimit(int(req.Msg.Limit))

	var lastCreatedAt *time.Time
	if req.Msg.LastCreatedAt != nil {
		t := req.Msg.LastCreatedAt.AsTime()
		lastCreatedAt = &t
	}

	articles, nextCreatedAt, nextID, err := h.listUnsummarized.ListUnsummarizedArticles(ctx, lastCreatedAt, req.Msg.LastId, limit)
	if err != nil {
		h.logger.Error("ListUnsummarizedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list unsummarized articles"))
	}

	protoArticles := make([]*backendv1.UnsummarizedArticle, len(articles))
	for i, a := range articles {
		protoArticles[i] = &backendv1.UnsummarizedArticle{
			Id:        a.ID,
			Title:     a.Title,
			Content:   a.Content,
			Url:       a.URL,
			CreatedAt: timestamppb.New(a.CreatedAt),
			UserId:    a.UserID,
		}
	}

	resp := &backendv1.ListUnsummarizedArticlesResponse{
		Articles: protoArticles,
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) HasUnsummarizedArticles(ctx context.Context, _ *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
	if h.hasUnsummarized == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	has, err := h.hasUnsummarized.HasUnsummarizedArticles(ctx)
	if err != nil {
		h.logger.Error("HasUnsummarizedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check unsummarized articles"))
	}

	return connect.NewResponse(&backendv1.HasUnsummarizedArticlesResponse{
		HasUnsummarized: has,
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
