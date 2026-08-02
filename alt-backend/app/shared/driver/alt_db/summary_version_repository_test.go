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
//
// Concretely: each call's UPDATE says
// `WHERE article_id = $2 AND superseded_by IS NULL AND summary_version_id != $1`,
// excluding only its *own* new id. Two concurrent calls therefore target
// disjoint row sets — each one's target is the other's new row — so the two
// UPDATEs never contend for a row and Postgres serializes nothing. They pass
// straight through each other and both commit, and the article is left with
// every version superseded and none current.
//
// ADR-000954 Wave 3 batch 5 is why this test now guards a process boundary as
// well as a race. MarkSummaryVersionSuperseded became one RPC —
// services.datahub.v1.DataHubService/MarkSummaryVersionSuperseded — with this whole
// transaction on the provider's side of it. That shape is forced rather than
// chosen: pg_advisory_xact_lock is released at COMMIT, so a contract that
// exposed the read and the update as two procedures would release the lock
// between them and reintroduce exactly the interleaving above, over a wider
// window than the in-process version ever had. The order of expectations below
// — BeginTx, advisory lock, select, update, Commit — is the contract that
// makes that true.
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
