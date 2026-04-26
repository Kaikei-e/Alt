// Package internal provides the Connect-RPC handler for BackendInternalService.
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	backendv1 "alt/gen/proto/services/backend/v1"
	"alt/gen/proto/services/backend/v1/backendv1connect"
	"alt/port/event_publisher_port"
	"alt/port/internal_article_port"
	"alt/port/internal_feed_port"
	"alt/port/internal_tag_port"
	"alt/port/knowledge_event_port"
	"alt/usecase/create_summary_version_usecase"
	"alt/usecase/create_tag_set_version_usecase"
	"alt/usecase/recap_articles_usecase"
	"fmt"
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
	upsertArticleTags        internal_tag_port.UpsertArticleTagsPort
	batchUpsertArticleTags   internal_tag_port.BatchUpsertArticleTagsPort
	listUntaggedArticles     internal_tag_port.ListUntaggedArticlesPort
	batchGetTagsByArticleIDs internal_tag_port.BatchGetTagsByArticleIDsPort

	// Phase 4 (quality checker)
	deleteArticleSummary      internal_article_port.DeleteArticleSummaryPort
	checkArticleSummaryExists internal_article_port.CheckArticleSummaryExistsPort
	findArticlesWithSummaries internal_article_port.FindArticlesWithSummariesPort

	// Summarization (pre-processor polling)
	listUnsummarized internal_article_port.ListUnsummarizedArticlesPort
	hasUnsummarized  internal_article_port.HasUnsummarizedArticlesPort

	// Backfill (pre-processor split-DB)
	getEmptyFeedID internal_feed_port.GetEmptyFeedIDPort

	// RAG Tool Operations (ADR-000617)
	fetchTagCloudPort      fetchTagCloudPort
	fetchArticlesByTagPort fetchArticlesByTagPort

	// Recap article window (recap-worker paginated fetch).
	recapArticlesUsecase recapArticlesUsecase

	// Event publishing
	eventPublisher     event_publisher_port.EventPublisherPort
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort

	// Knowledge version usecases
	createSummaryVersionUsecase *create_summary_version_usecase.CreateSummaryVersionUsecase
	createTagSetVersionUsecase  *create_tag_set_version_usecase.CreateTagSetVersionUsecase

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

// WithBatchGetTagsPort wires the BatchGetTagsByArticleIDs port used by
// recap-worker for tag fetches. Replaces the legacy tag-generator
// /api/v1/tags/batch path (ADR-000241 / ADR-000397).
func WithBatchGetTagsPort(p internal_tag_port.BatchGetTagsByArticleIDsPort) HandlerOption {
	return func(h *Handler) {
		h.batchGetTagsByArticleIDs = p
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

// WithBackfillPorts configures ports for backfill-related RPCs.
func WithBackfillPorts(
	getEmptyFeedID internal_feed_port.GetEmptyFeedIDPort,
) HandlerOption {
	return func(h *Handler) {
		h.getEmptyFeedID = getEmptyFeedID
	}
}

// WithKnowledgeEventPort configures the Knowledge Home event append port.
func WithKnowledgeEventPort(port knowledge_event_port.AppendKnowledgeEventPort) HandlerOption {
	return func(h *Handler) {
		h.knowledgeEventPort = port
	}
}

// WithKnowledgeVersionUsecases configures usecases for Knowledge Home version tracking.
func WithKnowledgeVersionUsecases(
	summaryVersion *create_summary_version_usecase.CreateSummaryVersionUsecase,
	tagSetVersion *create_tag_set_version_usecase.CreateTagSetVersionUsecase,
) HandlerOption {
	return func(h *Handler) {
		h.createSummaryVersionUsecase = summaryVersion
		h.createTagSetVersionUsecase = tagSetVersion
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

	articleID, created, err := h.createArticle.CreateArticle(ctx, internal_article_port.CreateArticleParams{
		Title:       req.Msg.Title,
		URL:         req.Msg.Url,
		Content:     req.Msg.Content,
		FeedID:      req.Msg.FeedId,
		UserID:      req.Msg.UserId,
		Language:    req.Msg.Language,
		PublishedAt: publishedAt,
	})
	if err != nil {
		h.logger.Error("CreateArticle failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create article"))
	}

	// Fire-and-forget: append Knowledge Home ArticleCreated event to sovereign-db
	if h.knowledgeEventPort != nil && created {
		if userID, parseErr := uuid.Parse(req.Msg.UserId); parseErr == nil {
			payload, _ := json.Marshal(map[string]string{
				"article_id":   articleID,
				"title":        req.Msg.Title,
				"published_at": publishedAt.Format(time.RFC3339),
				"tenant_id":    req.Msg.UserId,
				"link":         req.Msg.Url,
			})
			kevent := domain.KnowledgeEvent{
				EventID:       uuid.New(),
				OccurredAt:    time.Now(),
				TenantID:      userID,
				UserID:        &userID,
				ActorType:     domain.ActorService,
				ActorID:       "pre-processor",
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   articleID,
				DedupeKey:     fmt.Sprintf("article-created:%s", articleID),
				Payload:       payload,
			}
			if appendErr := h.knowledgeEventPort.AppendKnowledgeEvent(ctx, kevent); appendErr != nil {
				h.logger.Warn("failed to append knowledge ArticleCreated event (non-fatal)",
					"article_id", articleID, "error", appendErr)
			}
		}
	}

	// Fire-and-forget: publish ArticleCreated event for downstream consumers
	if h.eventPublisher != nil && h.eventPublisher.IsEnabled() {
		if created {
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
		} else if pubErr := h.eventPublisher.PublishArticleUpdated(ctx, event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   articleID,
			UserID:      req.Msg.UserId,
			FeedID:      req.Msg.FeedId,
			Title:       req.Msg.Title,
			URL:         req.Msg.Url,
			Content:     req.Msg.Content,
			PublishedAt: publishedAt,
		}); pubErr != nil {
			h.logger.Warn("failed to publish ArticleUpdated event (non-fatal)",
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

	// Also create summary version + knowledge event for Knowledge Home
	if h.createSummaryVersionUsecase != nil {
		articleUUID, parseErr := uuid.Parse(req.Msg.ArticleId)
		userUUID, userParseErr := uuid.Parse(req.Msg.UserId)
		if parseErr == nil && userParseErr == nil {
			sv := domain.SummaryVersion{
				ArticleID:   articleUUID,
				UserID:      userUUID,
				SummaryText: req.Msg.Summary,
				Model:       "pre-processor",
			}
			// Capture the article title at event-emission time so the
			// Knowledge Loop projector's reproject-safe enricher can render a
			// real card narrative (e.g. "{title} — fresh summary ready to
			// read.") instead of falling back to the generic feed-level
			// sentence. The lookup happens here at the handler boundary —
			// projection time stays pure (event payload only).
			if h.getArticleByID != nil {
				if article, lookupErr := h.getArticleByID.GetArticleByID(ctx, req.Msg.ArticleId); lookupErr == nil && article != nil {
					sv.ArticleTitle = article.Title
				}
			}
			if svErr := h.createSummaryVersionUsecase.Execute(ctx, sv); svErr != nil {
				h.logger.Error("failed to create summary version", "error", svErr, "article_id", req.Msg.ArticleId)
			}
		}
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

	// Also create tag set version + knowledge event for Knowledge Home
	if h.createTagSetVersionUsecase != nil && len(tags) > 0 {
		articleUUID, parseErr := uuid.Parse(req.Msg.ArticleId)
		if parseErr == nil {
			tagsJSON, _ := json.Marshal(tags)
			tsv := domain.TagSetVersion{
				ArticleID: articleUUID,
				Generator: "tag-generator",
				TagsJSON:  tagsJSON,
			}
			// Resolve UserID from article
			if h.getArticleByID != nil {
				article, artErr := h.getArticleByID.GetArticleByID(ctx, req.Msg.ArticleId)
				if artErr == nil && article != nil {
					userUUID, uErr := uuid.Parse(article.UserID)
					if uErr == nil {
						tsv.UserID = userUUID
					}
				} else {
					h.logger.Warn("could not resolve user for tag set version", "article_id", req.Msg.ArticleId, "error", artErr)
				}
			}
			if tsvErr := h.createTagSetVersionUsecase.Execute(ctx, tsv); tsvErr != nil {
				h.logger.Error("failed to create tag set version", "error", tsvErr, "article_id", req.Msg.ArticleId)
			}
		}
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

	// Create tag set version + knowledge event for each article
	if h.createTagSetVersionUsecase != nil {
		for _, item := range req.Msg.Items {
			if len(item.Tags) == 0 {
				continue
			}
			if item.FeedId == "" {
				h.logger.Warn("skipping tag set version creation for article without feed_id", "article_id", item.ArticleId)
				continue
			}
			articleUUID, parseErr := uuid.Parse(item.ArticleId)
			if parseErr != nil {
				continue
			}
			batchTags := make([]internal_tag_port.TagItem, len(item.Tags))
			for j, t := range item.Tags {
				batchTags[j] = internal_tag_port.TagItem{Name: t.Name, Confidence: t.Confidence}
			}
			tagsJSON, _ := json.Marshal(batchTags)
			tsv := domain.TagSetVersion{
				ArticleID: articleUUID,
				Generator: "tag-generator",
				TagsJSON:  tagsJSON,
			}
			// Resolve UserID from article
			if h.getArticleByID != nil {
				article, artErr := h.getArticleByID.GetArticleByID(ctx, item.ArticleId)
				if artErr == nil && article != nil {
					userUUID, uErr := uuid.Parse(article.UserID)
					if uErr == nil {
						tsv.UserID = userUUID
					}
				}
			}
			if tsvErr := h.createTagSetVersionUsecase.Execute(ctx, tsv); tsvErr != nil {
				h.logger.Error("failed to create tag set version in batch", "error", tsvErr, "article_id", item.ArticleId)
			}
		}
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

	protoArticles := make([]*backendv1.ArticleWithTags, len(articles))
	for i, a := range articles {
		var feedID string
		if a.FeedID != nil {
			feedID = *a.FeedID
		}
		protoArticles[i] = &backendv1.ArticleWithTags{
			Id:        a.ID,
			Title:     a.Title,
			Content:   a.Content,
			UserId:    a.UserID,
			FeedId:    feedID,
			CreatedAt: timestamppb.New(a.CreatedAt),
		}
	}

	resp := &backendv1.ListUntaggedArticlesResponse{
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
func (h *Handler) BatchGetTagsByArticleIDs(ctx context.Context, req *connect.Request[backendv1.BatchGetTagsByArticleIDsRequest]) (*connect.Response[backendv1.BatchGetTagsByArticleIDsResponse], error) {
	if h.batchGetTagsByArticleIDs == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	ids := req.Msg.GetArticleIds()
	if len(ids) == 0 {
		return connect.NewResponse(&backendv1.BatchGetTagsByArticleIDsResponse{}), nil
	}
	if len(ids) > maxBatchGetTagsArticleIDs {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("article_ids exceeds max batch size %d", maxBatchGetTagsArticleIDs))
	}

	grouped, err := h.batchGetTagsByArticleIDs.BatchGetTagsByArticleIDs(ctx, ids)
	if err != nil {
		h.logger.Error("BatchGetTagsByArticleIDs failed", "error", err, "article_count", len(ids))
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to batch fetch article tags"))
	}

	items := make([]*backendv1.ArticleTagsEntry, 0, len(grouped))
	for _, g := range grouped {
		tags := make([]*backendv1.ArticleTagEntry, len(g.Tags))
		for i, t := range g.Tags {
			tags[i] = &backendv1.ArticleTagEntry{
				TagName:    t.TagName,
				Confidence: t.Confidence,
				Source:     t.Source,
				UpdatedAt:  timestamppb.New(t.UpdatedAt),
			}
		}
		items = append(items, &backendv1.ArticleTagsEntry{
			ArticleId: g.ArticleID,
			Tags:      tags,
		})
	}

	return connect.NewResponse(&backendv1.BatchGetTagsByArticleIDsResponse{Items: items}), nil
}

// maxBatchGetTagsArticleIDs mirrors the caller-side guard previously
// enforced by tag-generator/app/auth_service.py. Kept as a server-side
// invariant so the driver never issues an unbounded ANY($1::uuid[]).
const maxBatchGetTagsArticleIDs = 1000

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

// ── Backfill operations (pre-processor split-DB) ──

func (h *Handler) GetEmptyFeedID(ctx context.Context, req *connect.Request[backendv1.GetEmptyFeedIDRequest]) (*connect.Response[backendv1.GetEmptyFeedIDResponse], error) {
	if h.getEmptyFeedID == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("feed_url is required"))
	}

	feedID, err := h.getEmptyFeedID.GetEmptyFeedID(ctx, req.Msg.FeedUrl)
	if err != nil {
		h.logger.Error("GetEmptyFeedID failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get empty feed ID"))
	}

	return connect.NewResponse(&backendv1.GetEmptyFeedIDResponse{
		FeedId: feedID,
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
		Language:  a.Language,
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

// --- RAG Tool Operations (ADR-000617) ---

// fetchTagCloudPort is the port interface for fetching tag cloud data.
type fetchTagCloudPort interface {
	Execute(ctx context.Context, limit int) ([]*domain.TagCloudItem, error)
}

// fetchArticlesByTagPort is the port interface for fetching articles by tag name.
type fetchArticlesByTagPort interface {
	ExecuteByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
}

// WithRAGToolPorts configures ports for RAG tool RPCs (ADR-000617).
func WithRAGToolPorts(
	tagCloud fetchTagCloudPort,
	articlesByTag fetchArticlesByTagPort,
) HandlerOption {
	return func(h *Handler) {
		h.fetchTagCloudPort = tagCloud
		h.fetchArticlesByTagPort = articlesByTag
	}
}

// recapArticlesUsecase is the minimal interface for paginated article window
// fetch. The concrete usecase lives at alt/usecase/recap_articles_usecase.
type recapArticlesUsecase interface {
	Execute(ctx context.Context, input recap_articles_usecase.Input) (*domain.RecapArticlesPage, error)
}

// WithRecapArticlesUsecase wires the recap-worker's paginated article window
// fetch (service-to-service RPC ListRecapArticles).
func WithRecapArticlesUsecase(uc recapArticlesUsecase) HandlerOption {
	return func(h *Handler) {
		h.recapArticlesUsecase = uc
	}
}

// ListRecapArticles returns paginated articles in a time window for the
// recap-worker. Authentication is enforced at the TLS transport layer
// (mTLS peer identity on :9443); this RPC intentionally does not require
// an end-user auth token.
func (h *Handler) ListRecapArticles(
	ctx context.Context,
	req *connect.Request[backendv1.ListRecapArticlesRequest],
) (*connect.Response[backendv1.ListRecapArticlesResponse], error) {
	if h.recapArticlesUsecase == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("ListRecapArticles not configured"))
	}

	msg := req.Msg
	if msg == nil || strings.TrimSpace(msg.From) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("from is required"))
	}
	if strings.TrimSpace(msg.To) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("to is required"))
	}

	from, err := time.Parse(time.RFC3339, msg.From)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("from must be RFC3339: %w", err))
	}
	to, err := time.Parse(time.RFC3339, msg.To)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("to must be RFC3339: %w", err))
	}

	var langHint *string
	if msg.LangHint != nil {
		lower := strings.ToLower(strings.TrimSpace(*msg.LangHint))
		if lower != "" {
			langHint = &lower
		}
	}

	input := recap_articles_usecase.Input{
		From:     from.UTC(),
		To:       to.UTC(),
		LangHint: langHint,
		Fields:   msg.Fields,
	}
	if msg.Page != nil {
		input.Page = int(*msg.Page)
	}
	if msg.PageSize != nil {
		input.PageSize = int(*msg.PageSize)
	}

	page, err := h.recapArticlesUsecase.Execute(ctx, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if page == nil {
		page = &domain.RecapArticlesPage{Page: input.Page, PageSize: input.PageSize}
	}

	articles := make([]*backendv1.RecapArticleItem, 0, len(page.Articles))
	for _, a := range page.Articles {
		item := &backendv1.RecapArticleItem{
			ArticleId: a.ID.String(),
			Fulltext:  a.FullText,
		}
		if a.Title != nil {
			item.Title = a.Title
		}
		if a.SourceURL != nil {
			item.SourceUrl = a.SourceURL
		}
		if a.LangHint != nil {
			item.LangHint = a.LangHint
		}
		if a.PublishedAt != nil {
			formatted := a.PublishedAt.UTC().Format(time.RFC3339)
			item.PublishedAt = &formatted
		}
		articles = append(articles, item)
	}

	resp := &backendv1.ListRecapArticlesResponse{
		Range:    &backendv1.RecapArticleRange{From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339)},
		Total:    int32(page.Total),
		Page:     int32(page.Page),
		PageSize: int32(page.PageSize),
		HasMore:  page.HasMore,
		Articles: articles,
	}
	return connect.NewResponse(resp), nil
}

// FetchTagCloud returns tag cloud data for topic exploration.
func (h *Handler) FetchTagCloud(ctx context.Context, req *connect.Request[backendv1.BackendInternalServiceFetchTagCloudRequest]) (*connect.Response[backendv1.BackendInternalServiceFetchTagCloudResponse], error) {
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

	tags := make([]*backendv1.TagCloudInternalItem, 0, len(items))
	for _, item := range items {
		tags = append(tags, &backendv1.TagCloudInternalItem{
			TagName:      item.TagName,
			ArticleCount: int32(item.ArticleCount),
		})
	}

	return connect.NewResponse(&backendv1.BackendInternalServiceFetchTagCloudResponse{
		Tags: tags,
	}), nil
}

// FetchArticlesByTag returns articles matching a tag name.
func (h *Handler) FetchArticlesByTag(ctx context.Context, req *connect.Request[backendv1.BackendInternalServiceFetchArticlesByTagRequest]) (*connect.Response[backendv1.BackendInternalServiceFetchArticlesByTagResponse], error) {
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

	result := make([]*backendv1.ArticleByTagItem, 0, len(articles))
	for _, a := range articles {
		result = append(result, &backendv1.ArticleByTagItem{
			Id:          a.ID,
			Title:       a.Title,
			Url:         a.Link,
			PublishedAt: a.PublishedAt.Format(time.RFC3339),
		})
	}

	return connect.NewResponse(&backendv1.BackendInternalServiceFetchArticlesByTagResponse{
		Articles: result,
	}), nil
}
