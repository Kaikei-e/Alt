package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"alt/dataplane/usecase/feeds_in_window_usecase"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// ListRecapArticles returns paginated articles in a time window for the
// recap-worker. Authentication is enforced at the TLS transport layer
// (mTLS peer identity on :9443); this RPC intentionally does not require
// an end-user auth token.
func (h *Handler) ListRecapArticles(
	ctx context.Context,
	req *connect.Request[datahubv1.ListRecapArticlesRequest],
) (*connect.Response[datahubv1.ListRecapArticlesResponse], error) {
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

	articles := make([]*datahubv1.RecapArticleItem, 0, len(page.Articles))
	for _, a := range page.Articles {
		item := &datahubv1.RecapArticleItem{
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

	resp := &datahubv1.ListRecapArticlesResponse{
		Range:    &datahubv1.RecapArticleRange{From: from.UTC().Format(time.RFC3339), To: to.UTC().Format(time.RFC3339)},
		Total:    safeconv.Int32(page.Total),
		Page:     safeconv.Int32(page.Page),
		PageSize: safeconv.Int32(page.PageSize),
		HasMore:  page.HasMore,
		Articles: articles,
	}
	return connect.NewResponse(resp), nil
}

// ListFeedsInWindow returns paginated RSS feed items in a time window for the
// recap-worker. Authentication is enforced at the TLS transport layer
// (mTLS peer identity on :9443); this RPC intentionally does not require
// an end-user auth token.
func (h *Handler) ListFeedsInWindow(
	ctx context.Context,
	req *connect.Request[datahubv1.ListFeedsInWindowRequest],
) (*connect.Response[datahubv1.ListFeedsInWindowResponse], error) {
	if h.feedsInWindowUsecase == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("ListFeedsInWindow not configured"))
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

	if msg.Page != nil && *msg.Page < 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("page must be >= 1"))
	}

	input := feeds_in_window_usecase.Input{
		From: from.UTC(),
		To:   to.UTC(),
	}
	if msg.Page != nil {
		input.Page = int(*msg.Page)
	}
	if msg.PageSize != nil {
		input.PageSize = int(*msg.PageSize)
	}

	page, err := h.feedsInWindowUsecase.Execute(ctx, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if page == nil {
		page = &domain.FeedsInWindowPage{Page: input.Page, PageSize: input.PageSize}
	}

	feeds := make([]*datahubv1.Feed, 0, len(page.Feeds))
	for i := range page.Feeds {
		if f := feedRowToProto(&page.Feeds[i]); f != nil {
			feeds = append(feeds, f)
		}
	}

	resp := &datahubv1.ListFeedsInWindowResponse{
		Feeds:    feeds,
		Total:    safeconv.Int32(page.Total),
		Page:     safeconv.Int32(page.Page),
		PageSize: safeconv.Int32(page.PageSize),
		HasMore:  page.HasMore,
	}
	return connect.NewResponse(resp), nil
}
