package sovereign_db

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetRecallCandidates_ResurfacesAfterSnoozeExpires pins the fix for the
// permanent-snooze bug: the original filter excluded any row where
// snoozed_until IS NULL, which after a snooze is set means the candidate
// never resurfaces even once snoozed_until has passed (snooze became a
// de-facto permanent dismiss). The fix allows resurfacing once the snooze
// window has elapsed.
func TestGetRecallCandidates_ResurfacesAfterSnoozeExpires(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetRecallCandidates(context.Background(), uuid.New(), 10)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	assert.Contains(t, sql, "(rcv.snoozed_until IS NULL OR rcv.snoozed_until <= now())",
		"snooze filter must allow candidates to resurface once snoozed_until has passed")
}

// TestGetKnowledgeHomeItems_SelectsProjectionVersion pins a field that was
// filtered on but never read back.
//
// `khi.projection_version` appeared only in the WHERE clause, so
// KnowledgeHomeItem.ProjectionVersion was never scanned and stayed 0 — and
// protojson omits a zero int32, so `projection_version` simply vanished from
// every GetKnowledgeHomeItems response even though sovereign.proto declares it.
// The sibling read one function down (GetRecallCandidates) does select
// rcv.projection_version, which is what makes this an asymmetry rather than a
// deliberate omission.
//
// Caught by e2e/playwright/knowledge-sovereign, whose schema requires the
// field: no unit test looked at the SELECT list, and no unit test could have
// noticed a field that is absent rather than wrong.
func TestGetKnowledgeHomeItems_SelectsProjectionVersion(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, _, _, err := repo.GetKnowledgeHomeItems(context.Background(), uuid.New(), "", 10, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	// Assert against the SELECT list alone. The WHERE clause filters on
	// `khi.projection_version = ...`, so a bare Contains over the whole
	// statement passes while the column is still unread — the same
	// can't-fail shape this test exists to catch.
	selectList, _, found := strings.Cut(sql, "FROM knowledge_home_items")
	require.True(t, found, "GetKnowledgeHomeItems no longer reads knowledge_home_items")

	assert.Contains(t, selectList, "khi.projection_version",
		"projection_version must be in the SELECT list, not only the WHERE clause — "+
			"the response message declares it and a client cannot tell 0 from absent")
}
