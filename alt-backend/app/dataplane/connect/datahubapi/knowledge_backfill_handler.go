package datahubapi

import (
	"context"
	"errors"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// ---------------------------------------------------------------------------
// §2.N Knowledge backfill reads
// ---------------------------------------------------------------------------

func (h *Handler) CountBackfillArticles(ctx context.Context, _ *connect.Request[datahubv1.CountBackfillArticlesRequest]) (*connect.Response[datahubv1.CountBackfillArticlesResponse], error) {
	count, err := h.knowledgeBackfill.CountArticles(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "CountBackfillArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count backfill articles"))
	}
	return connect.NewResponse(&datahubv1.CountBackfillArticlesResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) ListBackfillArticles(ctx context.Context, req *connect.Request[datahubv1.ListBackfillArticlesRequest]) (*connect.Response[datahubv1.ListBackfillArticlesResponse], error) {
	lastCreatedAt, lastArticleID, err := keysetCursor(req.Msg.GetLastCreatedAt(), req.Msg.GetLastArticleId(), "last_created_at", "last_article_id")
	if err != nil {
		return nil, err
	}

	articles, err := h.knowledgeBackfill.ListArticles(ctx, lastCreatedAt, lastArticleID, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListBackfillArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list backfill articles"))
	}

	out := make([]*datahubv1.BackfillArticle, 0, len(articles))
	for _, a := range articles {
		out = append(out, &datahubv1.BackfillArticle{
			ArticleId:   a.ArticleID.String(),
			UserId:      a.UserID.String(),
			CreatedAt:   timestampOrNil(a.CreatedAt),
			PublishedAt: timestampOrNil(a.PublishedAt),
			Title:       a.Title,
			Url:         a.URL,
		})
	}
	return connect.NewResponse(&datahubv1.ListBackfillArticlesResponse{Articles: out}), nil
}

func (h *Handler) CountBackfillSummaryTitles(ctx context.Context, _ *connect.Request[datahubv1.CountBackfillSummaryTitlesRequest]) (*connect.Response[datahubv1.CountBackfillSummaryTitlesResponse], error) {
	count, err := h.knowledgeBackfill.CountSummaryTitles(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "CountBackfillSummaryTitles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to count backfill summary titles"))
	}
	return connect.NewResponse(&datahubv1.CountBackfillSummaryTitlesResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) ListBackfillSummaryTitles(ctx context.Context, req *connect.Request[datahubv1.ListBackfillSummaryTitlesRequest]) (*connect.Response[datahubv1.ListBackfillSummaryTitlesResponse], error) {
	lastGeneratedAt, lastVersionID, err := keysetCursor(req.Msg.GetLastGeneratedAt(), req.Msg.GetLastSummaryVersionId(), "last_generated_at", "last_summary_version_id")
	if err != nil {
		return nil, err
	}

	entries, err := h.knowledgeBackfill.ListSummaryTitles(ctx, lastGeneratedAt, lastVersionID, clampLimit(int(req.Msg.GetLimit())))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListBackfillSummaryTitles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list backfill summary titles"))
	}

	out := make([]*datahubv1.BackfillSummaryTitle, 0, len(entries))
	for _, e := range entries {
		out = append(out, &datahubv1.BackfillSummaryTitle{
			SummaryVersionId: e.SummaryVersionID.String(),
			ArticleId:        e.ArticleID.String(),
			UserId:           e.UserID.String(),
			TenantId:         e.TenantID.String(),
			Title:            e.Title,
			GeneratedAt:      timestampOrNil(e.GeneratedAt),
		})
	}
	return connect.NewResponse(&datahubv1.ListBackfillSummaryTitlesResponse{Entries: out}), nil
}
