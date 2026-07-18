package sovereign_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetTrailFootprints_CollapsesRepeatedContacts pins the D24 read shape:
// the spine query groups raw footprints by (item_key, verb) so repeated
// contacts with one article collapse into a single row carrying the contact
// count and the first/latest contact times. The wear CTE keeps counting raw
// rows (a revisit still deepens the path).
func TestGetTrailFootprints_CollapsesRepeatedContacts(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, _, _, err := repo.GetTrailFootprints(context.Background(), uuid.New(), "", 20, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one spine query")

	sql := mock.queryCalls[0].SQL
	assert.Contains(t, sql, "count(*) AS contact_count",
		"repeated contacts must be counted, not repeated as rows")
	assert.Contains(t, sql, "min(occurred_at) AS first_occurred_at",
		"the earliest contact must survive the collapse")
	assert.Contains(t, sql, "max(occurred_at) AS occurred_at",
		"the collapsed row must sort by its latest contact")
	assert.Contains(t, sql, "GROUP BY tenant_id, item_key, verb",
		"the collapse key is (item_key, verb) within the user's spine")
	assert.Contains(t, sql, "GROUP BY item_key",
		"path wear must still fold over raw footprint rows")
}

// TestGetOpenTrailBranchesForAnchor_FiltersByAnchorAndLimits pins the Wave 10
// (D26) patch-exit read shape: branches narrow to one anchor item and are
// capped server-side, mirroring GetOpenTrailBranches but scoped to the
// article the user just finished reading.
func TestGetOpenTrailBranchesForAnchor_FiltersByAnchorAndLimits(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	_, err := repo.GetOpenTrailBranchesForAnchor(context.Background(), userID, "article:1", 2)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1, "expected one anchored-branches query")

	call := mock.queryCalls[0]
	assert.Contains(t, call.SQL, "b.anchor_item_key = $2",
		"the query must narrow to the anchor item, not the whole open set")
	assert.Contains(t, call.SQL, "b.state = 'open'",
		"only open branches surface")
	assert.Contains(t, call.SQL, "LIMIT $3",
		"the patch-exit surface is capped server-side (D26 — max 1-2)")
	require.Len(t, call.Args, 3)
	assert.Equal(t, userID, call.Args[0])
	assert.Equal(t, "article:1", call.Args[1])
	assert.Equal(t, 2, call.Args[2])
}

// TestGetItemTitle_QueriesByUserAndItemKey pins the small read the trail
// planner uses to resolve an anchor's display title (D28 — anchored why): a
// single-row lookup scoped to the user and item key.
func TestGetItemTitle_QueriesByUserAndItemKey(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	var gotSQL string
	var gotArgs []interface{}
	mock.queryRowFunc = func(_ context.Context, sql string, args ...interface{}) pgx.Row {
		gotSQL = sql
		gotArgs = args
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*string); ok {
				*p = "Async Rust"
			}
			return nil
		}}
	}

	userID := uuid.New()
	title, ok, err := repo.GetItemTitle(context.Background(), userID, "article:1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Async Rust", title)
	assert.Contains(t, gotSQL, "knowledge_home_items")
	assert.Contains(t, gotSQL, "item_key = $2")
	require.Len(t, gotArgs, 2)
	assert.Equal(t, userID, gotArgs[0])
	assert.Equal(t, "article:1", gotArgs[1])
}

// TestGetItemTitle_NoRowsReturnsNotOK pins the "cannot name it" branch: a
// missing row must report ok=false, not a fabricated title or an error the
// planner would have to distinguish from a real failure.
func TestGetItemTitle_NoRowsReturnsNotOK(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			return pgx.ErrNoRows
		}}
	}
	repo := &Repository{pool: mock}

	title, ok, err := repo.GetItemTitle(context.Background(), uuid.New(), "article:missing")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, title)
}
