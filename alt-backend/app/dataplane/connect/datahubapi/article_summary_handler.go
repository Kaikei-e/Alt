package datahubapi

import (
	"context"
	"errors"
	"time"

	"alt/dataplane/port/internal_article_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *Handler) SaveArticleSummary(ctx context.Context, req *connect.Request[datahubv1.SaveArticleSummaryRequest]) (*connect.Response[datahubv1.SaveArticleSummaryResponse], error) {
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

	err := h.saveArticleSummary.SaveArticleSummary(ctx, internal_article_port.SaveArticleSummaryParams{
		ArticleID: req.Msg.GetArticleId(),
		UserID:    req.Msg.GetUserId(),
		// Empty for pre-processor, which does not know the title. Set by
		// alt-backend's summarise paths, which do (ADR-000954 Wave 3 batch 5).
		ArticleTitle: req.Msg.GetArticleTitle(),
		Summary:      req.Msg.GetSummary(),
		Language:     req.Msg.GetLanguage(),
	})
	if err != nil {
		h.logger.Error("SaveArticleSummary failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to save article summary"))
	}

	// Also create summary version + knowledge event for Knowledge Home —
	// unless the caller says it owns its own versioning.
	//
	// Chaining the version onto this write is right for pre-processor, which
	// has no other way to record one, and wrong for a caller that appends its
	// own: alt-backend's stream-summarise path does exactly that, under its own
	// model name, and would otherwise get two versions and two
	// SummaryVersionCreated events for one summary. The only symptom would be a
	// duplicated entry in the Knowledge Home timeline, which no test on either
	// side reads. SUMMARY_VERSIONING_SKIP makes the caller say so.
	if req.Msg.GetSummaryVersioning() != datahubv1.SummaryVersioning_SUMMARY_VERSIONING_SKIP &&
		h.createSummaryVersionUsecase != nil {
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
			// Execute writes the summary_versions row and then appends
			// SummaryVersionCreated, so a failure can leave a version row that
			// no projection ever sees and no repair path visits. The RPC
			// result is the only acknowledgement this write has — callers read
			// nothing but Success — so it is what gets withheld, exactly as on
			// the ArticleCreated path. Unavailable rather than Internal
			// because the caller's correct response is to send the summary
			// again: the re-send supersedes the orphaned version and appends
			// the event that was lost.
			if svErr := h.createSummaryVersionUsecase.Execute(ctx, sv); svErr != nil {
				h.logger.Error("failed to create summary version; refusing to acknowledge the write",
					"error", svErr, "article_id", req.Msg.ArticleId)
				return nil, connect.NewError(connect.CodeUnavailable,
					errors.New("summary written but its version could not be recorded; retry"))
			}
		}
	}

	return connect.NewResponse(&datahubv1.SaveArticleSummaryResponse{
		Success: true,
	}), nil
}

// ── Summary quality operations (quality checker) ──

func (h *Handler) DeleteArticleSummary(ctx context.Context, req *connect.Request[datahubv1.DeleteArticleSummaryRequest]) (*connect.Response[datahubv1.DeleteArticleSummaryResponse], error) {
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

	return connect.NewResponse(&datahubv1.DeleteArticleSummaryResponse{
		Success: true,
	}), nil
}

func (h *Handler) CheckArticleSummaryExists(ctx context.Context, req *connect.Request[datahubv1.CheckArticleSummaryExistsRequest]) (*connect.Response[datahubv1.CheckArticleSummaryExistsResponse], error) {
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

	return connect.NewResponse(&datahubv1.CheckArticleSummaryExistsResponse{
		Exists:    exists,
		SummaryId: summaryID,
	}), nil
}

func (h *Handler) FindArticlesWithSummaries(ctx context.Context, req *connect.Request[datahubv1.FindArticlesWithSummariesRequest]) (*connect.Response[datahubv1.FindArticlesWithSummariesResponse], error) {
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

	protoArticles := make([]*datahubv1.ArticleWithSummaryItem, len(articles))
	for i, a := range articles {
		protoArticles[i] = &datahubv1.ArticleWithSummaryItem{
			ArticleId:       a.ArticleID,
			ArticleContent:  a.ArticleContent,
			ArticleUrl:      a.ArticleURL,
			SummaryId:       a.SummaryID,
			SummaryJapanese: a.SummaryJapanese,
			CreatedAt:       timestamppb.New(a.CreatedAt),
		}
	}

	resp := &datahubv1.FindArticlesWithSummariesResponse{
		Articles: protoArticles,
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

// ── Summarization operations (pre-processor polling) ──

func (h *Handler) ListUnsummarizedArticles(ctx context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
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

	protoArticles := make([]*datahubv1.UnsummarizedArticle, len(articles))
	for i, a := range articles {
		protoArticles[i] = &datahubv1.UnsummarizedArticle{
			Id:        a.ID,
			Title:     a.Title,
			Content:   a.Content,
			Url:       a.URL,
			CreatedAt: timestamppb.New(a.CreatedAt),
			UserId:    a.UserID,
		}
	}

	resp := &datahubv1.ListUnsummarizedArticlesResponse{
		Articles: protoArticles,
		NextId:   nextID,
	}
	if nextCreatedAt != nil {
		resp.NextCreatedAt = timestamppb.New(*nextCreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) HasUnsummarizedArticles(ctx context.Context, _ *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
	if h.hasUnsummarized == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	has, err := h.hasUnsummarized.HasUnsummarizedArticles(ctx)
	if err != nil {
		h.logger.Error("HasUnsummarizedArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to check unsummarized articles"))
	}

	return connect.NewResponse(&datahubv1.HasUnsummarizedArticlesResponse{
		HasUnsummarized: has,
	}), nil
}

func (h *Handler) GetArticleSummaryByArticleID(ctx context.Context, req *connect.Request[datahubv1.GetArticleSummaryByArticleIDRequest]) (*connect.Response[datahubv1.GetArticleSummaryByArticleIDResponse], error) {
	if req.Msg.GetArticleId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	userID, err := optionalUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	summary, err := h.feed.GetSummaryByArticleID(ctx, req.Msg.GetArticleId(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleSummaryByArticleID failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article summary"))
	}
	return connect.NewResponse(&datahubv1.GetArticleSummaryByArticleIDResponse{Summary: feedSummaryToProto(summary)}), nil
}
