package sovereign_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBackfillJob_ScansAllFourteenColumns pins the column-count bug found
// in the 2026-07-06 review: the SELECT lists 14 columns (including `kind`)
// but Scan only had 13 destinations, so pgx rejected every real call with a
// field-count mismatch. This structural test fails if a future edit drops
// a column back out of the Scan call without updating the SELECT (or vice
// versa).
func TestGetBackfillJob_ScansAllFourteenColumns(t *testing.T) {
	mock := &mockPgx{}
	wantKind := "articles"
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			require.Len(t, dest, 14, "GetBackfillJob Scan must have 14 destinations matching the 14-column SELECT")
			kindPtr, ok := dest[2].(*string)
			require.True(t, ok, "3rd scan destination (matching SELECT column order job_id, status, kind, ...) must be *string for kind")
			*kindPtr = wantKind
			return nil
		}}
	}

	repo := &Repository{pool: mock}
	job, err := repo.GetBackfillJob(context.Background(), uuid.New())
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, wantKind, job.Kind, "Kind must round-trip through Scan, not stay zero-valued")
}

// TestActivateProjectionVersion_RejectsUnknownVersionWithoutTouchingActive
// pins the fix for the zero-active-versions bug: an invalid version argument
// must be rejected BEFORE any active version is deactivated, so a bad call
// (or a mid-sequence crash) can never leave zero active projection versions
// (which would silently regress every reader's COALESCE(...,1) fallback).
func TestActivateProjectionVersion_RejectsUnknownVersionWithoutTouchingActive(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			return pgx.ErrNoRows // version does not exist
		}}
	}

	repo := &Repository{pool: mock}
	err := repo.ActivateProjectionVersion(context.Background(), 999)

	require.Error(t, err, "unknown version must be rejected")
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, mock.execCalls, "no UPDATE (deactivate or activate) may run when the target version doesn't exist")
	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.rolledBack, "the opened transaction must be rolled back on the existence-check failure")
	assert.False(t, mock.lastTx.committed, "must not commit when the target version doesn't exist")
}

// TestActivateProjectionVersion_DeactivateAndActivateAreAtomic pins the
// fix that the deactivate+activate pair now runs inside a single
// transaction (Begin...Commit) instead of two independent Exec calls —
// a mid-failure between them can no longer be observed as "zero active
// versions" by a concurrent reader.
func TestActivateProjectionVersion_DeactivateAndActivateAreAtomic(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*int); ok {
				*p = 1
			}
			return nil
		}}
	}
	mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}

	repo := &Repository{pool: mock}
	err := repo.ActivateProjectionVersion(context.Background(), 2)

	require.NoError(t, err)
	require.Len(t, mock.execCalls, 2, "expected exactly one deactivate and one activate UPDATE")
	assert.Contains(t, mock.execCalls[0].SQL, "status = 'inactive'")
	assert.Contains(t, mock.execCalls[1].SQL, "status = 'active'")
	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.committed, "both UPDATEs must be committed together")
}
