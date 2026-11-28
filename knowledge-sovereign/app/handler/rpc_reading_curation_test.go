package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestListReadingAnnotations_ValidAndInvalid(t *testing.T) {
	userID := uuid.New()
	annID := uuid.New()
	artID := uuid.New()
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		readingAnnotations: []sovereign_db.ReadingAnnotation{
			{
				UserID:       userID,
				AnnotationID: annID,
				ArticleID:    artID,
				Stance:       "surprise",
				Excerpt:      "Surprising claim",
				Note:         "Check primary source",
				CreatedAt:    now,
			},
		},
	}

	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.ListReadingAnnotations(context.Background(), connect.NewRequest(&sovereignv1.ListReadingAnnotationsRequest{
		UserId:     userID.String(),
		ArticleIds: []string{artID.String()},
		Limit:      20,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetAnnotations(), 1)
	assert.Equal(t, annID.String(), resp.Msg.GetAnnotations()[0].GetAnnotationId())
	assert.Equal(t, "surprise", resp.Msg.GetAnnotations()[0].GetStance())

	_, err = client.ListReadingAnnotations(context.Background(), connect.NewRequest(&sovereignv1.ListReadingAnnotationsRequest{
		UserId: "not-a-uuid",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestListReadingThemeCurations_ValidAndInvalid(t *testing.T) {
	userID := uuid.New()
	themeID := uuid.New()
	curID := uuid.New()
	now := time.Date(2026, 11, 28, 12, 30, 0, 0, time.UTC)

	repo := &mockRepo{
		readingThemeCurations: []sovereign_db.ReadingThemeCuration{
			{
				UserID:     userID,
				CurationID: curID,
				ThemeID:    themeID,
				ActionKind: "add_question",
				TargetText: "Does this hold at 10x scale?",
				UserNote:   "",
				CreatedAt:  now,
			},
		},
	}

	client, cleanup := setupTestServer(repo)
	defer cleanup()

	resp, err := client.ListReadingThemeCurations(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemeCurationsRequest{
		UserId:  userID.String(),
		ThemeId: themeID.String(),
		Limit:   50,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetCurations(), 1)
	assert.Equal(t, "add_question", resp.Msg.GetCurations()[0].GetActionKind())
	assert.Equal(t, "Does this hold at 10x scale?", resp.Msg.GetCurations()[0].GetTargetText())

	_, err = client.ListReadingThemeCurations(context.Background(), connect.NewRequest(&sovereignv1.ListReadingThemeCurationsRequest{
		UserId:  userID.String(),
		ThemeId: "bad",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
