package sovereign_db

import (
	"context"
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
