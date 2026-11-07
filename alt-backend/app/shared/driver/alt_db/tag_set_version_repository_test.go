package alt_db

import (
	"context"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// What a mock can hold onto here is the shape of the transaction: that the
// advisory lock is taken before anything is read, that the read of the
// incoming version's age comes first, and that nothing is written outside the
// transaction. Whether the resulting supersede chain is correct is decided by
// PostgreSQL, and lives in tag_set_version_repository_pg_test.go.
//
// The distinction matters because this file used to be the only coverage, and
// it passed throughout the period when two regenerations racing on one article
// left that article with no current tag set at all — the mock replayed the
// scripted rows and never noticed. See ADR-000954 Wave 3 batch 5 for why the
// whole transaction has to stay inside one RPC rather than splitting the read
// from the update.
func TestMarkTagSetVersionSuperseded_UsesTransactionWithAdvisoryLock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	prevID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))

	mock.ExpectQuery("SELECT generated_at FROM tag_set_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))

	// No newer version outstanding, so this one goes on to supersede.
	mock.ExpectQuery("SELECT tag_set_version_id FROM tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"tag_set_version_id"}))

	mock.ExpectQuery("SELECT tag_set_version_id, article_id, user_id").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{
			"tag_set_version_id", "article_id", "user_id", "generated_at",
			"generator", "input_hash", "tags_json", "superseded_by",
		}).AddRow(prevID, articleID, uuid.New(), generatedAt.Add(-time.Minute), "generator", "hash", []byte(`["a"]`), nil))

	mock.ExpectExec("UPDATE tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID, newVersionID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectCommit()

	prev, err := repo.MarkTagSetVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.NotNil(t, prev)
	require.Equal(t, prevID, prev.TagSetVersionID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkTagSetVersionSuperseded_NoPreviousVersion_CommitsEmptyTx(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM tag_set_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))
	mock.ExpectQuery("SELECT tag_set_version_id FROM tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"tag_set_version_id"}))
	mock.ExpectQuery("SELECT tag_set_version_id, article_id, user_id").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{
			"tag_set_version_id", "article_id", "user_id", "generated_at",
			"generator", "input_hash", "tags_json", "superseded_by",
		}))
	mock.ExpectCommit()

	prev, err := repo.MarkTagSetVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.Nil(t, prev)

	require.NoError(t, mock.ExpectationsWereMet())
}

// A version that lost to a newer one points at the winner instead of
// superseding anything, and reports no predecessor — otherwise the caller
// emits a TagSetSuperseded event for a replacement that never happened.
func TestMarkTagSetVersionSuperseded_YieldsToNewerVersion(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	winnerID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM tag_set_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))
	mock.ExpectQuery("SELECT tag_set_version_id FROM tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"tag_set_version_id"}).AddRow(winnerID))
	mock.ExpectExec("UPDATE tag_set_versions").
		WithArgs(winnerID, newVersionID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	prev, err := repo.MarkTagSetVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.Nil(t, prev)

	require.NoError(t, mock.ExpectationsWereMet())
}

// An id that names no row is a wiring fault. Answering it with a silent
// success would leave the article's tags frozen and say nothing.
func TestMarkTagSetVersionSuperseded_UnknownVersionIsAnError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM tag_set_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}))
	mock.ExpectRollback()

	_, err = repo.MarkTagSetVersionSuperseded(context.Background(), articleID, newVersionID)
	require.Error(t, err)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateTagSetVersion_IdempotentOnDuplicateVersionID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	tsv := domain.TagSetVersion{
		TagSetVersionID: uuid.New(),
		ArticleID:       uuid.New(),
		UserID:          uuid.New(),
		GeneratedAt:     time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		Generator:       "tag-generator",
		InputHash:       "hash",
		TagsJSON:        []byte(`[{"name":"go"}]`),
	}

	mock.ExpectExec("INSERT INTO tag_set_versions.*ON CONFLICT \\(tag_set_version_id\\) DO NOTHING").
		WithArgs(tsv.TagSetVersionID, tsv.ArticleID, tsv.UserID, tsv.GeneratedAt, tsv.Generator, tsv.InputHash, tsv.TagsJSON, tsv.SupersededBy).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))

	require.NoError(t, repo.CreateTagSetVersion(context.Background(), tsv))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkTagSetVersionSuperseded_RetryMatchesAlreadySupersededBySameVersion(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	prevID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM tag_set_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))
	mock.ExpectQuery("SELECT tag_set_version_id FROM tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"tag_set_version_id"}))
	mock.ExpectQuery("SELECT tag_set_version_id, article_id, user_id.*superseded_by IS NULL OR superseded_by = \\$3").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{
			"tag_set_version_id", "article_id", "user_id", "generated_at",
			"generator", "input_hash", "tags_json", "superseded_by",
		}).AddRow(prevID, articleID, uuid.New(), generatedAt.Add(-time.Minute), "generator", "hash", []byte(`["a"]`), &newVersionID))
	mock.ExpectExec("UPDATE tag_set_versions").
		WithArgs(articleID, generatedAt, newVersionID, newVersionID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	prev, err := repo.MarkTagSetVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.NotNil(t, prev)
	require.Equal(t, prevID, prev.TagSetVersionID)

	require.NoError(t, mock.ExpectationsWereMet())
}
