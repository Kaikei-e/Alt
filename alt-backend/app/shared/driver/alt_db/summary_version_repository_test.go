package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// Finding [4]: MarkSummaryVersionSuperseded must serialize concurrent calls
// for the same article via a transaction-scoped advisory lock, otherwise two
// summary versions created back-to-back for the same article can each mark
// the other superseded (mutual-supersede cycle), leaving
// GetLatestSummaryVersion unable to find any current version even though
// summaries were generated successfully. A per-article advisory lock
// (pg_advisory_xact_lock) held for the transaction's lifetime fully
// serializes the select+update pair, which row-level locking on the
// (changing, disjoint-by-construction) candidate row set cannot do.
func TestMarkSummaryVersionSuperseded_UsesTransactionWithAdvisoryLock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SummaryRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	prevID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))

	rows := pgxmock.NewRows([]string{
		"summary_version_id", "article_id", "user_id", "generated_at",
		"model", "prompt_version", "input_hash", "quality_score",
		"summary_text", "superseded_by",
	}).AddRow(prevID, articleID, uuid.New(), now, "model", "v1", "hash", (*float64)(nil), "text", nil)

	mock.ExpectQuery("SELECT summary_version_id, article_id").
		WithArgs(articleID, newVersionID).
		WillReturnRows(rows)

	mock.ExpectExec("UPDATE summary_versions").
		WithArgs(newVersionID, articleID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectCommit()

	prev, err := repo.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.NotNil(t, prev)
	require.Equal(t, prevID, prev.SummaryVersionID)

	require.NoError(t, mock.ExpectationsWereMet())
}

// No previous version exists: the transaction must still commit (not be left
// open / rolled back) and return (nil, nil).
func TestMarkSummaryVersionSuperseded_NoPreviousVersion_CommitsEmptyTx(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SummaryRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT summary_version_id, article_id").
		WithArgs(articleID, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{
			"summary_version_id", "article_id", "user_id", "generated_at",
			"model", "prompt_version", "input_hash", "quality_score",
			"summary_text", "superseded_by",
		}))
	mock.ExpectCommit()

	prev, err := repo.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.Nil(t, prev)

	require.NoError(t, mock.ExpectationsWereMet())
}
