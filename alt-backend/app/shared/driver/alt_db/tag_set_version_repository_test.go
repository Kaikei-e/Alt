package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// Finding [4]: MarkTagSetVersionSuperseded is the tag_set_versions twin of
// MarkSummaryVersionSuperseded and carries the exact same mutual-supersede
// race — see summary_version_repository_test.go for the full mechanism, and
// for why ADR-000954 Wave 3 batch 5 had to keep the whole transaction inside
// one RPC rather than splitting the read from the update.
//
// The race is more likely here than there: tag sets are regenerated whenever
// an article is re-tagged, which happens on the on-the-fly generation path as
// well as in the batch job, so two calls for one article arriving together is
// ordinary rather than exceptional.
func TestMarkTagSetVersionSuperseded_UsesTransactionWithAdvisoryLock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	prevID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))

	rows := pgxmock.NewRows([]string{
		"tag_set_version_id", "article_id", "user_id", "generated_at",
		"generator", "input_hash", "tags_json", "superseded_by",
	}).AddRow(prevID, articleID, uuid.New(), now, "generator", "hash", []byte(`["a"]`), nil)

	mock.ExpectQuery("SELECT tag_set_version_id, article_id").
		WithArgs(articleID, newVersionID).
		WillReturnRows(rows)

	mock.ExpectExec("UPDATE tag_set_versions").
		WithArgs(newVersionID, articleID).
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

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT tag_set_version_id, article_id").
		WithArgs(articleID, newVersionID).
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
