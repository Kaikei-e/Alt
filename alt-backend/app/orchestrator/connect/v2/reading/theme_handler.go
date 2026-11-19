package reading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	readingv1 "alt/gen/proto/alt/reading/v1"
)

func (h *Handler) ListThemes(
	ctx context.Context,
	_ *connect.Request[readingv1.ListThemesRequest],
) (*connect.Response[readingv1.ListThemesResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	themes, err := h.listThemesUsecase.Execute(ctx, user.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidThemeInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ListThemes")
	}

	out := make([]*readingv1.ThemeSummary, 0, len(themes))
	for _, t := range themes {
		item := &readingv1.ThemeSummary{
			ThemeId:           t.ThemeID.String(),
			Title:             t.Title,
			LatestVersion:     t.LatestVersion,
			ArticleCount:      t.ArticleCount,
			LastOpenedVersion: t.LastOpenedVersion,
		}
		if !t.UpdatedAt.IsZero() {
			item.UpdatedAt = timestamppb.New(t.UpdatedAt)
		}
		if t.LastOpenedAt != nil && !t.LastOpenedAt.IsZero() {
			item.LastOpenedAt = timestamppb.New(*t.LastOpenedAt)
		}
		out = append(out, item)
	}

	return connect.NewResponse(&readingv1.ListThemesResponse{
		Themes: out,
	}), nil
}

func (h *Handler) GetTheme(
	ctx context.Context,
	req *connect.Request[readingv1.GetThemeRequest],
) (*connect.Response[readingv1.GetThemeResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var version *int32
	if req.Msg.Version != nil {
		v := *req.Msg.Version
		version = &v
	}

	page, err := h.getThemeUsecase.Execute(ctx, user.UserID, req.Msg.GetThemeId(), version)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidThemeInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if errors.Is(err, domain.ErrThemeNotFound) {
			return nil, errorhandler.HandleNotFoundError(ctx, h.logger, "theme not found", "GetTheme")
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetTheme")
	}

	resp := &readingv1.GetThemeResponse{
		ThemeId:       page.ThemeID,
		Title:         page.Title,
		Version:       page.Version,
		LatestVersion: page.LatestVersion,
		KeyPoints:     themeSentencesToProto(page.KeyPoints),
		Timeline:      themeTimelineToProto(page.Timeline),
		WhatChanged:   themeSentencesToProto(page.WhatChanged),
		OpenQuestions: themeSentencesToProto(page.OpenQuestions),
		Sources:       themeSourcesToProto(page.Sources),
		Versions:      themeVersionsToProto(page.Versions),
	}
	if !page.CreatedAt.IsZero() {
		resp.CreatedAt = timestamppb.New(page.CreatedAt)
	}

	return connect.NewResponse(resp), nil
}

func (h *Handler) RecordThemeOpened(
	ctx context.Context,
	req *connect.Request[readingv1.RecordThemeOpenedRequest],
) (*connect.Response[readingv1.RecordThemeOpenedResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var openedAt time.Time
	if ts := req.Msg.GetOpenedAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid opened_at", domain.ErrInvalidThemeInput))
		}
		openedAt = ts.AsTime()
	}

	if err := h.recordThemeOpenedUsecase.Execute(
		ctx,
		user.UserID,
		user.TenantID,
		req.Msg.GetThemeId(),
		req.Msg.GetVersion(),
		req.Msg.GetClientEventId(),
		openedAt,
	); err != nil {
		if errors.Is(err, domain.ErrInvalidThemeInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "RecordThemeOpened")
	}

	return connect.NewResponse(&readingv1.RecordThemeOpenedResponse{}), nil
}

func themeSentencesToProto(in []domain.ThemeSentence) []*readingv1.ThemeSentence {
	out := make([]*readingv1.ThemeSentence, 0, len(in))
	for _, s := range in {
		out = append(out, &readingv1.ThemeSentence{
			Text:             s.Text,
			SourceArticleIds: s.SourceArticleIDs,
		})
	}
	return out
}

func themeTimelineToProto(in []domain.ThemeTimelineEntry) []*readingv1.ThemeTimelineEntry {
	out := make([]*readingv1.ThemeTimelineEntry, 0, len(in))
	for _, t := range in {
		out = append(out, &readingv1.ThemeTimelineEntry{
			Date:             t.Date,
			Text:             t.Text,
			SourceArticleIds: t.SourceArticleIDs,
		})
	}
	return out
}

func themeSourcesToProto(in []domain.ThemeSource) []*readingv1.ThemeSource {
	out := make([]*readingv1.ThemeSource, 0, len(in))
	for _, s := range in {
		out = append(out, &readingv1.ThemeSource{
			ArticleId: s.ArticleID,
			Title:     s.Title,
			Url:       s.URL,
		})
	}
	return out
}

func themeVersionsToProto(in []domain.ThemeVersionRef) []*readingv1.ThemeVersionRef {
	out := make([]*readingv1.ThemeVersionRef, 0, len(in))
	for _, v := range in {
		ref := &readingv1.ThemeVersionRef{
			Version:      v.Version,
			ArticleCount: v.ArticleCount,
		}
		if !v.CreatedAt.IsZero() {
			ref.CreatedAt = timestamppb.New(v.CreatedAt)
		}
		out = append(out, ref)
	}
	return out
}
