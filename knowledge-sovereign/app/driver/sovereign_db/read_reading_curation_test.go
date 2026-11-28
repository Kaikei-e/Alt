package sovereign_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndDeleteReadingAnnotation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	annID := uuid.New()
	artID := uuid.New()
	createdAt := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO reading_annotations")).
		WithArgs(userID, tenantID, annID, artID, "question", "Quote from article", "Why does this happen?", createdAt, int64(42)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, repo.UpsertReadingAnnotation(context.Background(), ReadingAnnotation{
		UserID:       userID,
		TenantID:     tenantID,
		AnnotationID: annID,
		ArticleID:    artID,
		Stance:       "question",
		Excerpt:      "Quote from article",
		Note:         "Why does this happen?",
		CreatedAt:    createdAt,
		LastEventSeq: 42,
	}))

	deletedAt := createdAt.Add(time.Minute)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE reading_annotations")).
		WithArgs(userID, annID, deletedAt, int64(43)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.DeleteReadingAnnotation(context.Background(), userID, annID, deletedAt, 43))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListReadingAnnotations_FiltersByArticleIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	annID := uuid.New()
	artID := uuid.New()
	createdAt := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows([]string{
		"user_id", "tenant_id", "annotation_id", "article_id",
		"stance", "excerpt", "note", "created_at", "last_event_seq",
	}).AddRow(userID, tenantID, annID, artID, "insight", "Key quote", "My synthesis", createdAt, int64(10))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, tenant_id, annotation_id, article_id")).
		WithArgs(userID, []uuid.UUID{artID}, 25).
		WillReturnRows(rows)

	got, err := repo.ListReadingAnnotations(context.Background(), userID, []uuid.UUID{artID}, 25)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, annID, got[0].AnnotationID)
	assert.Equal(t, "insight", got[0].Stance)
	assert.Equal(t, "My synthesis", got[0].Note)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertAndListReadingThemeCurations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}
	userID := uuid.New()
	tenantID := uuid.New()
	curID := uuid.New()
	themeID := uuid.New()
	createdAt := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO reading_theme_curations")).
		WithArgs(userID, tenantID, curID, themeID, "pin_key_point", "hash123", "Important claim [1]", "Core principle", "", createdAt, int64(50)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, repo.UpsertReadingThemeCuration(context.Background(), ReadingThemeCuration{
		UserID:       userID,
		TenantID:     tenantID,
		CurationID:   curID,
		ThemeID:      themeID,
		ActionKind:   "pin_key_point",
		TargetHash:   "hash123",
		TargetText:   "Important claim [1]",
		UserNote:     "Core principle",
		CreatedAt:    createdAt,
		LastEventSeq: 50,
	}))

	rows := pgxmock.NewRows([]string{
		"user_id", "tenant_id", "curation_id", "theme_id",
		"action_kind", "target_hash", "target_text", "user_note",
		"augur_conversation_id", "created_at", "last_event_seq",
	}).AddRow(userID, tenantID, curID, themeID, "pin_key_point", "hash123", "Important claim [1]", "Core principle", "", createdAt, int64(50))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, tenant_id, curation_id, theme_id")).
		WithArgs(userID, themeID, 50).
		WillReturnRows(rows)

	got, err := repo.ListReadingThemeCurations(context.Background(), userID, themeID, 50)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "pin_key_point", got[0].ActionKind)
	assert.Equal(t, "Core principle", got[0].UserNote)
	require.NoError(t, mock.ExpectationsWereMet())
}
