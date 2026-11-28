package reading_curation_usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

type fakeAppendPort struct {
	events []domain.KnowledgeEvent
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	f.events = append(f.events, event)
	return int64(len(f.events)), nil
}

type fakeReadPort struct {
	annotations []domain.ArticleAnnotation
	curations   []domain.ThemeCuration
}

func (f *fakeReadPort) ListArticleAnnotations(_ context.Context, _ uuid.UUID, _ []uuid.UUID, _ int32) ([]domain.ArticleAnnotation, error) {
	return f.annotations, nil
}

func (f *fakeReadPort) ListThemeCurations(_ context.Context, _, _ uuid.UUID, _ int32) ([]domain.ThemeCuration, error) {
	return f.curations, nil
}

func TestSaveAndDeleteArticleAnnotation(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	port := &fakeAppendPort{}
	saveUC := NewSaveArticleAnnotationUsecase(port, func() time.Time { return now })
	delUC := NewDeleteArticleAnnotationUsecase(port, func() time.Time { return now })

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	clientEvtID := uuid.New()

	ann, err := saveUC.Execute(context.Background(), userID, tenantID, SaveAnnotationInput{
		ArticleID:     articleID.String(),
		Stance:        domain.StanceQuestion,
		Excerpt:       "  Key sentence\x00with control char  ",
		Note:          "How does this compare to B?",
		ClientEventID: clientEvtID.String(),
		CreatedAt:     now,
	})
	require.NoError(t, err)
	assert.Equal(t, clientEvtID, ann.AnnotationID)
	assert.Equal(t, "Key sentencewith control char", ann.Excerpt)
	assert.Equal(t, "How does this compare to B?", ann.Note)
	require.Len(t, port.events, 1)
	assert.Equal(t, domain.EventReadingAnnotationCreated, port.events[0].EventType)

	// Reject oversized text (> 500 runes)
	_, err = saveUC.Execute(context.Background(), userID, tenantID, SaveAnnotationInput{
		ArticleID:     articleID.String(),
		Stance:        domain.StanceInsight,
		Note:          strings.Repeat("あ", domain.MaxCurationTextRunes+1),
		ClientEventID: uuid.New().String(),
	})
	require.ErrorIs(t, err, domain.ErrInvalidAnnotationInput)

	// Reject empty excerpt + empty note
	_, err = saveUC.Execute(context.Background(), userID, tenantID, SaveAnnotationInput{
		ArticleID:     articleID.String(),
		Stance:        domain.StanceInsight,
		ClientEventID: uuid.New().String(),
	})
	require.ErrorIs(t, err, domain.ErrInvalidAnnotationInput)

	// Delete
	require.NoError(t, delUC.Execute(context.Background(), userID, tenantID, ann.AnnotationID.String(), uuid.New().String(), now))
	require.Len(t, port.events, 2)
	assert.Equal(t, domain.EventReadingAnnotationDeleted, port.events[1].EventType)
}

func TestCurateAndRemoveThemeItem(t *testing.T) {
	now := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	port := &fakeAppendPort{}
	curateUC := NewCurateThemeItemUsecase(port, func() time.Time { return now })
	removeUC := NewRemoveThemeCurationUsecase(port, func() time.Time { return now })

	userID := uuid.New()
	tenantID := uuid.New()
	themeID := uuid.New()
	convID := uuid.New()
	clientEvtID := uuid.New()

	cur, err := curateUC.Execute(context.Background(), userID, tenantID, CurateThemeItemInput{
		ThemeID:             themeID.String(),
		Kind:                domain.CurationPromoteAugurInsight,
		TargetText:          "Does consensus need synchronized clocks?",
		UserNote:            "No, logical clocks suffice for safety.",
		AugurConversationID: convID.String(),
		ClientEventID:       clientEvtID.String(),
		CreatedAt:           now,
	})
	require.NoError(t, err)
	assert.Equal(t, clientEvtID, cur.CurationID)
	assert.NotEmpty(t, cur.TargetHash)
	assert.Equal(t, convID.String(), cur.AugurConversationID)
	require.Len(t, port.events, 1)
	assert.Equal(t, domain.EventReadingThemeItemCurated, port.events[0].EventType)

	// Invalid Augur conversation ID must be rejected
	_, err = curateUC.Execute(context.Background(), userID, tenantID, CurateThemeItemInput{
		ThemeID:             themeID.String(),
		Kind:                domain.CurationPromoteAugurInsight,
		TargetText:          "Question",
		AugurConversationID: "not-a-uuid",
		ClientEventID:       uuid.New().String(),
	})
	require.ErrorIs(t, err, domain.ErrInvalidCurationInput)

	require.NoError(t, removeUC.Execute(context.Background(), userID, tenantID, cur.CurationID.String(), uuid.New().String(), now))
	require.Len(t, port.events, 2)
	assert.Equal(t, domain.EventReadingThemeCurationRemove, port.events[1].EventType)
}
