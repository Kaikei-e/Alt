package reading

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	readingv1 "alt/gen/proto/alt/reading/v1"
	"alt/orchestrator/usecase/reading_curation_usecase"
)

type fakeCurationReadPort struct {
	annotations []domain.ArticleAnnotation
	curations   []domain.ThemeCuration
}

func (f *fakeCurationReadPort) ListArticleAnnotations(_ context.Context, _ uuid.UUID, _ []uuid.UUID, _ int32) ([]domain.ArticleAnnotation, error) {
	return f.annotations, nil
}

func (f *fakeCurationReadPort) ListThemeCurations(_ context.Context, _, _ uuid.UUID, _ int32) ([]domain.ThemeCuration, error) {
	return f.curations, nil
}

func newCurationTestHandler(t *testing.T, readPort *fakeCurationReadPort) (*Handler, *fakeAppendPort) {
	t.Helper()
	appendPort := &fakeAppendPort{}
	clock := fixedClock()

	h := newThemeTestHandler(appendPort, &fakeListThemesPort{}, &fakeGetThemePort{}).WithCurationUsecases(
		reading_curation_usecase.NewSaveArticleAnnotationUsecase(appendPort, clock),
		reading_curation_usecase.NewDeleteArticleAnnotationUsecase(appendPort, clock),
		reading_curation_usecase.NewListArticleAnnotationsUsecase(readPort),
		reading_curation_usecase.NewCurateThemeItemUsecase(appendPort, clock),
		reading_curation_usecase.NewRemoveThemeCurationUsecase(appendPort, clock),
		reading_curation_usecase.NewListThemeCurationsUsecase(readPort),
	)
	return h, appendPort
}

func TestHandler_SaveListDeleteArticleAnnotation(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	artID := uuid.New()
	annID := uuid.New()
	readPort := &fakeCurationReadPort{
		annotations: []domain.ArticleAnnotation{
			{
				AnnotationID: annID,
				ArticleID:    artID,
				Stance:       domain.StanceInsight,
				Excerpt:      "Core claim",
				Note:         "Connects to Theme A",
				CreatedAt:    now,
			},
		},
	}
	h, appendPort := newCurationTestHandler(t, readPort)

	// Unauthenticated rejected
	_, err := h.SaveArticleAnnotation(context.Background(), connect.NewRequest(&readingv1.SaveArticleAnnotationRequest{
		ArticleId:     artID.String(),
		Stance:        readingv1.AnnotationStance_ANNOTATION_STANCE_INSIGHT,
		Note:          "test",
		ClientEventId: annID.String(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	// Save succeeds
	saveResp, err := h.SaveArticleAnnotation(authenticatedContext(t), connect.NewRequest(&readingv1.SaveArticleAnnotationRequest{
		ArticleId:     artID.String(),
		Stance:        readingv1.AnnotationStance_ANNOTATION_STANCE_QUESTION,
		Excerpt:       "Quote",
		Note:          "Why?",
		ClientEventId: annID.String(),
		CreatedAt:     timestamppb.New(now),
	}))
	require.NoError(t, err)
	assert.Equal(t, annID.String(), saveResp.Msg.GetAnnotation().GetAnnotationId())
	assert.Equal(t, readingv1.AnnotationStance_ANNOTATION_STANCE_QUESTION, saveResp.Msg.GetAnnotation().GetStance())
	require.Len(t, appendPort.events, 1)

	// List returns annotations
	listResp, err := h.ListArticleAnnotations(authenticatedContext(t), connect.NewRequest(&readingv1.ListArticleAnnotationsRequest{
		ArticleIds: []string{artID.String()},
		Limit:      20,
	}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.GetAnnotations(), 1)
	assert.Equal(t, "Connects to Theme A", listResp.Msg.GetAnnotations()[0].GetNote())

	// Delete succeeds
	_, err = h.DeleteArticleAnnotation(authenticatedContext(t), connect.NewRequest(&readingv1.DeleteArticleAnnotationRequest{
		AnnotationId:  annID.String(),
		ClientEventId: uuid.New().String(),
		DeletedAt:     timestamppb.New(now),
	}))
	require.NoError(t, err)
	require.Len(t, appendPort.events, 2)
}

func TestHandler_CurateListRemoveThemeItem(t *testing.T) {
	now := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	themeID := uuid.New()
	curID := uuid.New()
	convID := uuid.New()
	readPort := &fakeCurationReadPort{
		curations: []domain.ThemeCuration{
			{
				CurationID:          curID,
				ThemeID:             themeID,
				Kind:                domain.CurationPinKeyPoint,
				TargetHash:          "abcd1234",
				TargetText:          "Pinned claim [1]",
				UserNote:            "Confirmed",
				AugurConversationID: convID.String(),
				CreatedAt:           now,
			},
		},
	}
	h, appendPort := newCurationTestHandler(t, readPort)

	curResp, err := h.CurateThemeItem(authenticatedContext(t), connect.NewRequest(&readingv1.CurateThemeItemRequest{
		ThemeId:             themeID.String(),
		Kind:                readingv1.ThemeCurationKind_THEME_CURATION_KIND_PROMOTE_AUGUR_INSIGHT,
		TargetText:          "What is the root cause?",
		UserNote:            "Confirmed via Augur",
		AugurConversationId: convID.String(),
		ClientEventId:       curID.String(),
		CreatedAt:           timestamppb.New(now),
	}))
	require.NoError(t, err)
	assert.Equal(t, curID.String(), curResp.Msg.GetCuration().GetCurationId())
	assert.Equal(t, readingv1.ThemeCurationKind_THEME_CURATION_KIND_PROMOTE_AUGUR_INSIGHT, curResp.Msg.GetCuration().GetKind())
	require.Len(t, appendPort.events, 1)

	listResp, err := h.ListThemeCurations(authenticatedContext(t), connect.NewRequest(&readingv1.ListThemeCurationsRequest{
		ThemeId: themeID.String(),
		Limit:   50,
	}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.GetCurations(), 1)
	assert.Equal(t, readingv1.ThemeCurationKind_THEME_CURATION_KIND_PIN_KEY_POINT, listResp.Msg.GetCurations()[0].GetKind())

	_, err = h.RemoveThemeCuration(authenticatedContext(t), connect.NewRequest(&readingv1.RemoveThemeCurationRequest{
		CurationId:    curID.String(),
		ClientEventId: uuid.New().String(),
		RemovedAt:     timestamppb.New(now),
	}))
	require.NoError(t, err)
	require.Len(t, appendPort.events, 2)
}

func TestHandler_UnwiredCurationPanics(t *testing.T) {
	h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, &fakeGetThemePort{})

	assert.Panics(t, func() {
		_, _ = h.ListArticleAnnotations(authenticatedContext(t), connect.NewRequest(&readingv1.ListArticleAnnotationsRequest{}))
	})
}
