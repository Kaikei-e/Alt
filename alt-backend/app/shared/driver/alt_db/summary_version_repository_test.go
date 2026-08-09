package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// MarkSummaryVersionSuperseded needs two things at once, and this file can
// only witness the first.
//
// The first is serialization. Two summary versions created back-to-back for
// one article each run this method, and each one's select-then-update pair
// computes its candidate rows before the other's UPDATE lands, so row-level
// locking contends over nothing and Postgres serializes nothing. A per-article
// pg_advisory_xact_lock held for the transaction's lifetime is what actually
// orders them. The expectation order below — BeginTx, advisory lock, reads,
// update, Commit — is that contract.
//
// The second is that the ordered calls agree on who won. They did not: the
// UPDATE used to read `superseded_by IS NULL AND summary_version_id != $1`,
// excluding only its own id, so a call would supersede a version generated
// after itself. Serialized or not, two regenerations left every version
// pointing at a successor and the article with no current summary — which is
// the state GetLatestSummaryVersion cannot represent. The predicate now
// compares generation age, and a version that arrives after a newer one has
// won points at that winner instead.
//
// A mock cannot tell those two apart: it replays whatever rows the test hands
// it, so it reported success throughout the period the second property was
// broken. summary_version_repository_pg_test.go owns that half, against a real
// server. What stays here is the transaction's shape.
//
// ADR-000954 Wave 3 batch 5 is why the shape guards a process boundary too.
// This became one RPC — services.datahub.v1.DataHubService/MarkSummaryVersionSuperseded
// — with the whole transaction on the provider's side. That is forced rather
// than chosen: pg_advisory_xact_lock is released at COMMIT, so a contract
// exposing the read and the update as two procedures would drop the lock
// between them and reopen the interleaving over a wider window than the
// in-process version ever had.
func TestMarkSummaryVersionSuperseded_UsesTransactionWithAdvisoryLock(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SummaryRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	prevID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))

	mock.ExpectQuery("SELECT generated_at FROM summary_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))

	// No newer version outstanding, so this one goes on to supersede.
	mock.ExpectQuery("SELECT summary_version_id FROM summary_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"summary_version_id"}))

	mock.ExpectQuery("SELECT summary_version_id, article_id, user_id").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{
			"summary_version_id", "article_id", "user_id", "generated_at",
			"model", "prompt_version", "input_hash", "quality_score",
			"summary_text", "superseded_by",
		}).AddRow(prevID, articleID, uuid.New(), generatedAt.Add(-time.Minute), "model", "v1", "hash", (*float64)(nil), "text", nil))

	mock.ExpectExec("UPDATE summary_versions").
		WithArgs(articleID, generatedAt, newVersionID, newVersionID).
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
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM summary_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))
	mock.ExpectQuery("SELECT summary_version_id FROM summary_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"summary_version_id"}))
	mock.ExpectQuery("SELECT summary_version_id, article_id, user_id").
		WithArgs(articleID, generatedAt, newVersionID).
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

// A version that lost to a newer one points at the winner instead of
// superseding anything, and reports no predecessor — otherwise the caller
// emits a SummarySuperseded event for a replacement that never happened.
func TestMarkSummaryVersionSuperseded_YieldsToNewerVersion(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &SummaryRepository{pool: mock}

	articleID := uuid.New()
	newVersionID := uuid.New()
	winnerID := uuid.New()
	generatedAt := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("pg_advisory_xact_lock").
		WithArgs(articleID).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
	mock.ExpectQuery("SELECT generated_at FROM summary_versions").
		WithArgs(newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"generated_at"}).AddRow(generatedAt))
	mock.ExpectQuery("SELECT summary_version_id FROM summary_versions").
		WithArgs(articleID, generatedAt, newVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"summary_version_id"}).AddRow(winnerID))
	mock.ExpectExec("UPDATE summary_versions").
		WithArgs(winnerID, newVersionID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	prev, err := repo.MarkSummaryVersionSuperseded(context.Background(), articleID, newVersionID)
	require.NoError(t, err)
	require.Nil(t, prev)

	require.NoError(t, mock.ExpectationsWereMet())
}
