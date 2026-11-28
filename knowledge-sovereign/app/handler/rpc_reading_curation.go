package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

const (
	defaultReadingAnnotationsLimit = 100
	maxReadingAnnotationsLimit     = 500
	defaultThemeCurationsLimit     = 200
	maxThemeCurationsLimit         = 500
)

// ListReadingAnnotations returns active user micro-externalizations optionally
// scoped to a list of article IDs, ordered newest first.
func (h *SovereignHandler) ListReadingAnnotations(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListReadingAnnotationsRequest],
) (*connect.Response[sovereignv1.ListReadingAnnotationsResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	articleIDs := make([]uuid.UUID, 0, len(msg.GetArticleIds()))
	for _, raw := range msg.GetArticleIds() {
		id, parseErr := parseUUIDField("article_ids", raw)
		if parseErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, parseErr)
		}
		articleIDs = append(articleIDs, id)
	}

	limit := int(msg.GetLimit())
	if limit <= 0 {
		limit = defaultReadingAnnotationsLimit
	} else if limit > maxReadingAnnotationsLimit {
		limit = maxReadingAnnotationsLimit
	}

	rows, err := h.readDB.ListReadingAnnotations(ctx, userID, articleIDs, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListReadingAnnotations: %w", err))
	}

	entries := make([]*sovereignv1.ReadingAnnotationEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, &sovereignv1.ReadingAnnotationEntry{
			AnnotationId: r.AnnotationID.String(),
			ArticleId:    r.ArticleID.String(),
			Stance:       r.Stance,
			Excerpt:      r.Excerpt,
			Note:         r.Note,
			CreatedAt:    timestamppb.New(r.CreatedAt),
		})
	}
	return connect.NewResponse(&sovereignv1.ListReadingAnnotationsResponse{
		Annotations: entries,
	}), nil
}

// ListReadingThemeCurations returns active user co-creation curations on one
// theme, ordered newest first.
func (h *SovereignHandler) ListReadingThemeCurations(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListReadingThemeCurationsRequest],
) (*connect.Response[sovereignv1.ListReadingThemeCurationsResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	themeID, err := parseUUIDField("theme_id", msg.GetThemeId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	limit := int(msg.GetLimit())
	if limit <= 0 {
		limit = defaultThemeCurationsLimit
	} else if limit > maxThemeCurationsLimit {
		limit = maxThemeCurationsLimit
	}

	rows, err := h.readDB.ListReadingThemeCurations(ctx, userID, themeID, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListReadingThemeCurations: %w", err))
	}

	entries := make([]*sovereignv1.ReadingThemeCurationEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, &sovereignv1.ReadingThemeCurationEntry{
			CurationId:          r.CurationID.String(),
			ThemeId:             r.ThemeID.String(),
			ActionKind:          r.ActionKind,
			TargetHash:          r.TargetHash,
			TargetText:          r.TargetText,
			UserNote:            r.UserNote,
			AugurConversationId: r.AugurConversationID,
			CreatedAt:           timestamppb.New(r.CreatedAt),
		})
	}
	return connect.NewResponse(&sovereignv1.ListReadingThemeCurationsResponse{
		Curations: entries,
	}), nil
}
