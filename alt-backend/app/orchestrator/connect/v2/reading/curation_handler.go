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
	"alt/orchestrator/usecase/reading_curation_usecase"
)

// WithCurationUsecases wires the micro-externalization and theme co-creation
// usecases onto the ReadingService handler. Panics if any dependency is nil
// (Rule 8: no silent fallback for unwired dependencies).
func (h *Handler) WithCurationUsecases(
	saveAnn *reading_curation_usecase.SaveArticleAnnotationUsecase,
	delAnn *reading_curation_usecase.DeleteArticleAnnotationUsecase,
	listAnn *reading_curation_usecase.ListArticleAnnotationsUsecase,
	curateTheme *reading_curation_usecase.CurateThemeItemUsecase,
	removeCur *reading_curation_usecase.RemoveThemeCurationUsecase,
	listCur *reading_curation_usecase.ListThemeCurationsUsecase,
) *Handler {
	if saveAnn == nil || delAnn == nil || listAnn == nil || curateTheme == nil || removeCur == nil || listCur == nil {
		panic("reading: all 6 curation usecases are required")
	}
	h.saveAnnotationUsecase = saveAnn
	h.deleteAnnotationUsecase = delAnn
	h.listAnnotationsUsecase = listAnn
	h.curateThemeUsecase = curateTheme
	h.removeThemeCurationUsecase = removeCur
	h.listThemeCurationsUsecase = listCur
	h.logger.Info("reading_curation_enabled")
	return h
}

func (h *Handler) SaveArticleAnnotation(
	ctx context.Context,
	req *connect.Request[readingv1.SaveArticleAnnotationRequest],
) (*connect.Response[readingv1.SaveArticleAnnotationResponse], error) {
	if h.saveAnnotationUsecase == nil {
		panic("reading: SaveArticleAnnotation reached with unwired SaveArticleAnnotationUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	stance, ok := annotationStanceFromProto(req.Msg.GetStance())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid stance", domain.ErrInvalidAnnotationInput))
	}
	var createdAt time.Time
	if ts := req.Msg.GetCreatedAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid created_at", domain.ErrInvalidAnnotationInput))
		}
		createdAt = ts.AsTime()
	}

	ann, err := h.saveAnnotationUsecase.Execute(ctx, user.UserID, user.TenantID, reading_curation_usecase.SaveAnnotationInput{
		ArticleID:     req.Msg.GetArticleId(),
		Stance:        stance,
		Excerpt:       req.Msg.GetExcerpt(),
		Note:          req.Msg.GetNote(),
		ClientEventID: req.Msg.GetClientEventId(),
		CreatedAt:     createdAt,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAnnotationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "SaveArticleAnnotation")
	}
	return connect.NewResponse(&readingv1.SaveArticleAnnotationResponse{
		Annotation: articleAnnotationToProto(*ann),
	}), nil
}

func (h *Handler) DeleteArticleAnnotation(
	ctx context.Context,
	req *connect.Request[readingv1.DeleteArticleAnnotationRequest],
) (*connect.Response[readingv1.DeleteArticleAnnotationResponse], error) {
	if h.deleteAnnotationUsecase == nil {
		panic("reading: DeleteArticleAnnotation reached with unwired DeleteArticleAnnotationUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	var deletedAt time.Time
	if ts := req.Msg.GetDeletedAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid deleted_at", domain.ErrInvalidAnnotationInput))
		}
		deletedAt = ts.AsTime()
	}

	if err := h.deleteAnnotationUsecase.Execute(
		ctx,
		user.UserID,
		user.TenantID,
		req.Msg.GetAnnotationId(),
		req.Msg.GetClientEventId(),
		deletedAt,
	); err != nil {
		if errors.Is(err, domain.ErrInvalidAnnotationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "DeleteArticleAnnotation")
	}
	return connect.NewResponse(&readingv1.DeleteArticleAnnotationResponse{}), nil
}

func (h *Handler) ListArticleAnnotations(
	ctx context.Context,
	req *connect.Request[readingv1.ListArticleAnnotationsRequest],
) (*connect.Response[readingv1.ListArticleAnnotationsResponse], error) {
	if h.listAnnotationsUsecase == nil {
		panic("reading: ListArticleAnnotations reached with unwired ListArticleAnnotationsUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	rows, err := h.listAnnotationsUsecase.Execute(ctx, user.UserID, req.Msg.GetArticleIds(), req.Msg.GetLimit())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAnnotationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ListArticleAnnotations")
	}
	out := make([]*readingv1.ArticleAnnotation, 0, len(rows))
	for _, r := range rows {
		out = append(out, articleAnnotationToProto(r))
	}
	return connect.NewResponse(&readingv1.ListArticleAnnotationsResponse{
		Annotations: out,
	}), nil
}

func (h *Handler) CurateThemeItem(
	ctx context.Context,
	req *connect.Request[readingv1.CurateThemeItemRequest],
) (*connect.Response[readingv1.CurateThemeItemResponse], error) {
	if h.curateThemeUsecase == nil {
		panic("reading: CurateThemeItem reached with unwired CurateThemeItemUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	kind, ok := themeCurationKindFromProto(req.Msg.GetKind())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid curation kind", domain.ErrInvalidCurationInput))
	}
	var createdAt time.Time
	if ts := req.Msg.GetCreatedAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid created_at", domain.ErrInvalidCurationInput))
		}
		createdAt = ts.AsTime()
	}

	cur, err := h.curateThemeUsecase.Execute(ctx, user.UserID, user.TenantID, reading_curation_usecase.CurateThemeItemInput{
		ThemeID:             req.Msg.GetThemeId(),
		Kind:                kind,
		TargetHash:          req.Msg.GetTargetHash(),
		TargetText:          req.Msg.GetTargetText(),
		UserNote:            req.Msg.GetUserNote(),
		AugurConversationID: req.Msg.GetAugurConversationId(),
		ClientEventID:       req.Msg.GetClientEventId(),
		CreatedAt:           createdAt,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCurationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "CurateThemeItem")
	}
	return connect.NewResponse(&readingv1.CurateThemeItemResponse{
		Curation: themeCurationToProto(*cur),
	}), nil
}

func (h *Handler) RemoveThemeCuration(
	ctx context.Context,
	req *connect.Request[readingv1.RemoveThemeCurationRequest],
) (*connect.Response[readingv1.RemoveThemeCurationResponse], error) {
	if h.removeThemeCurationUsecase == nil {
		panic("reading: RemoveThemeCuration reached with unwired RemoveThemeCurationUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	var removedAt time.Time
	if ts := req.Msg.GetRemovedAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid removed_at", domain.ErrInvalidCurationInput))
		}
		removedAt = ts.AsTime()
	}

	if err := h.removeThemeCurationUsecase.Execute(
		ctx,
		user.UserID,
		user.TenantID,
		req.Msg.GetCurationId(),
		req.Msg.GetClientEventId(),
		removedAt,
	); err != nil {
		if errors.Is(err, domain.ErrInvalidCurationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "RemoveThemeCuration")
	}
	return connect.NewResponse(&readingv1.RemoveThemeCurationResponse{}), nil
}

func (h *Handler) ListThemeCurations(
	ctx context.Context,
	req *connect.Request[readingv1.ListThemeCurationsRequest],
) (*connect.Response[readingv1.ListThemeCurationsResponse], error) {
	if h.listThemeCurationsUsecase == nil {
		panic("reading: ListThemeCurations reached with unwired ListThemeCurationsUsecase (DI gap)")
	}
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	rows, err := h.listThemeCurationsUsecase.Execute(ctx, user.UserID, req.Msg.GetThemeId(), req.Msg.GetLimit())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCurationInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ListThemeCurations")
	}
	out := make([]*readingv1.ThemeCuration, 0, len(rows))
	for _, r := range rows {
		out = append(out, themeCurationToProto(r))
	}
	return connect.NewResponse(&readingv1.ListThemeCurationsResponse{
		Curations: out,
	}), nil
}

func annotationStanceFromProto(s readingv1.AnnotationStance) (domain.AnnotationStance, bool) {
	switch s {
	case readingv1.AnnotationStance_ANNOTATION_STANCE_SURPRISE:
		return domain.StanceSurprise, true
	case readingv1.AnnotationStance_ANNOTATION_STANCE_QUESTION:
		return domain.StanceQuestion, true
	case readingv1.AnnotationStance_ANNOTATION_STANCE_CONTRADICTION:
		return domain.StanceContradiction, true
	case readingv1.AnnotationStance_ANNOTATION_STANCE_INSIGHT:
		return domain.StanceInsight, true
	default:
		return "", false
	}
}

func annotationStanceToProto(s domain.AnnotationStance) readingv1.AnnotationStance {
	switch s {
	case domain.StanceSurprise:
		return readingv1.AnnotationStance_ANNOTATION_STANCE_SURPRISE
	case domain.StanceQuestion:
		return readingv1.AnnotationStance_ANNOTATION_STANCE_QUESTION
	case domain.StanceContradiction:
		return readingv1.AnnotationStance_ANNOTATION_STANCE_CONTRADICTION
	case domain.StanceInsight:
		return readingv1.AnnotationStance_ANNOTATION_STANCE_INSIGHT
	default:
		return readingv1.AnnotationStance_ANNOTATION_STANCE_UNSPECIFIED
	}
}

func articleAnnotationToProto(a domain.ArticleAnnotation) *readingv1.ArticleAnnotation {
	out := &readingv1.ArticleAnnotation{
		AnnotationId: a.AnnotationID.String(),
		ArticleId:    a.ArticleID.String(),
		Stance:       annotationStanceToProto(a.Stance),
		Excerpt:      a.Excerpt,
		Note:         a.Note,
	}
	if !a.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(a.CreatedAt)
	}
	return out
}

func themeCurationKindFromProto(k readingv1.ThemeCurationKind) (domain.ThemeCurationKind, bool) {
	switch k {
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_PIN_KEY_POINT:
		return domain.CurationPinKeyPoint, true
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_DISMISS_KEY_POINT:
		return domain.CurationDismissKeyPoint, true
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_ADD_QUESTION:
		return domain.CurationAddQuestion, true
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_RESOLVE_QUESTION:
		return domain.CurationResolveQuestion, true
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_ADD_SEED_NOTE:
		return domain.CurationAddSeedNote, true
	case readingv1.ThemeCurationKind_THEME_CURATION_KIND_PROMOTE_AUGUR_INSIGHT:
		return domain.CurationPromoteAugurInsight, true
	default:
		return "", false
	}
}

func themeCurationKindToProto(k domain.ThemeCurationKind) readingv1.ThemeCurationKind {
	switch k {
	case domain.CurationPinKeyPoint:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_PIN_KEY_POINT
	case domain.CurationDismissKeyPoint:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_DISMISS_KEY_POINT
	case domain.CurationAddQuestion:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_ADD_QUESTION
	case domain.CurationResolveQuestion:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_RESOLVE_QUESTION
	case domain.CurationAddSeedNote:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_ADD_SEED_NOTE
	case domain.CurationPromoteAugurInsight:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_PROMOTE_AUGUR_INSIGHT
	default:
		return readingv1.ThemeCurationKind_THEME_CURATION_KIND_UNSPECIFIED
	}
}

func themeCurationToProto(c domain.ThemeCuration) *readingv1.ThemeCuration {
	out := &readingv1.ThemeCuration{
		CurationId:          c.CurationID.String(),
		ThemeId:             c.ThemeID.String(),
		Kind:                themeCurationKindToProto(c.Kind),
		TargetHash:          c.TargetHash,
		TargetText:          c.TargetText,
		UserNote:            c.UserNote,
		AugurConversationId: c.AugurConversationID,
	}
	if !c.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(c.CreatedAt)
	}
	return out
}
