package sovereign_db

import (
	"context"
	"strings"
	"testing"
	"time"

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

// TestGetKnowledgeHomeItems_OrdersByReadTimeRecencyDecay pins the fix for the
// frozen-ranking defect: score used to be a snapshot of freshness computed
// once at ArticleCreated fold time (occurredAt - published_at) and then
// merge-safe-UPSERTed with GREATEST, so the stored score could never
// reflect an item's actual growing staleness — a slot won at ingest was
// held forever. Ranking must instead apply a recency decay over
// published_at at READ time (every query re-evaluates "how old is this
// now"), while the stored khi.score itself is left untouched (still a
// time-invariant quality/affinity signal — see baseQualityScore in the
// projector). A bare `ORDER BY khi.score DESC` is exactly the regression
// this test catches: it ranks purely on the frozen snapshot again.
func TestGetKnowledgeHomeItems_OrdersByReadTimeRecencyDecay(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, _, _, err := repo.GetKnowledgeHomeItems(context.Background(), uuid.New(), "", 10, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	assert.NotContains(t, sql, "ORDER BY khi.score DESC",
		"ranking must not order by the raw stored score alone — that is the frozen-snapshot bug "+
			"(score is fixed at ArticleCreated fold time and only ever ratchets up)")
	assert.Contains(t, sql, "now() - COALESCE(khi.published_at, khi.generated_at)",
		"ranking must recompute recency against the current time on every read, not rely on a value baked in at fold time")
}

// TestGetKnowledgeHomeItems_CursorContinuationUsesSameRankExpressionAsOrderBy
// pins that keyset pagination stays consistent with the ORDER BY: if the
// WHERE continuation clause compared against raw khi.score while ORDER BY
// ranked by the decayed expression, pages would skip or repeat rows whenever
// the two disagreed on ordering.
//
// A prior version of this test decoded the cursor with the pre-anchor
// 3-field encodeCursor and asserted the WHERE clause contained the literal
// "now() - COALESCE(khi.published_at, khi.generated_at)". strings.Cut(sql,
// "ORDER BY") splits at the FIRST "ORDER BY" — the one embedded inside
// activeProjectionVersionSQL's subquery, which sits in the WHERE clause
// before the cursor predicate — so whereClause was actually just the SELECT
// list plus that subquery, and the assertion passed off the SELECT list's
// own rank_score expression regardless of what the cursor predicate
// contained. It would have passed even with no cursor predicate at all.
//
// This version isolates the clause between the top-level "FROM
// knowledge_home_items khi" and the LAST "ORDER BY" (the real one, which
// always terminates the query) — excluding the SELECT list entirely, since
// the SELECT list's own rank_score expression would otherwise satisfy the
// assertion regardless of what the cursor predicate says. Mutation-proven:
// this narrower slice still passed when only sql[:lastOrderBy] was used
// (see mutationProofs) because that range still included the SELECT list.
func TestGetKnowledgeHomeItems_CursorContinuationUsesSameRankExpressionAsOrderBy(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	anchor := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	cursor := encodeCursor(0.42, nil, "article:cursor-item", anchor)
	_, _, _, err := repo.GetKnowledgeHomeItems(context.Background(), uuid.New(), cursor, 10, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	fromIdx := strings.Index(sql, "FROM knowledge_home_items khi")
	require.NotEqual(t, -1, fromIdx, "expected the top-level FROM knowledge_home_items khi")
	lastOrderBy := strings.LastIndex(sql, "ORDER BY")
	require.NotEqual(t, -1, lastOrderBy, "expected an ORDER BY clause")
	require.Greater(t, lastOrderBy, fromIdx, "ORDER BY must come after FROM")
	whereClause, orderByClause := sql[fromIdx:lastOrderBy], sql[lastOrderBy:]

	assert.Contains(t, whereClause, "COALESCE(khi.published_at, khi.generated_at)",
		"the keyset continuation predicate must compare against the same decayed rank expression as ORDER BY")
	assert.Contains(t, orderByClause, "rank_score",
		"ORDER BY must reference the decayed rank expression, whether via alias or restated")
}

// TestGetKnowledgeHomeItems_CursorContinuationReusesAnchoredNowFromCursor
// pins the fix for the each_key_duplicate defect: homeItemRankScoreSQL
// decays strictly with elapsed time, so if a continuation page re-evaluated
// live now() instead of reusing the instant the FIRST page anchored on
// (carried in the cursor), the page-boundary row's rank_score would come
// out strictly lower than the cursor's recorded value on every later
// request. Since the keyset predicate is a strict "<", that row would then
// satisfy its own predicate and be re-emitted on every subsequent page,
// crashing the Knowledge Home stream's keyed {#each}.
func TestGetKnowledgeHomeItems_CursorContinuationReusesAnchoredNowFromCursor(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	anchor := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	cursor := encodeCursor(0.42, nil, "article:cursor-item", anchor)
	_, _, _, err := repo.GetKnowledgeHomeItems(context.Background(), uuid.New(), cursor, 10, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL
	args := mock.queryCalls[0].Args

	assert.NotContains(t, sql, "now()",
		"a continuation page must anchor decay to the cursor's captured instant, not call now() again — "+
			"otherwise every later page ranks the boundary row lower than its own cursor value and re-emits it forever")
	assert.Contains(t, args, anchor,
		"the instant decoded from the cursor must be bound as a query parameter so ORDER BY/WHERE decay against it")
}

// TestGetKnowledgeHomeItems_FirstPageAnchorsOnDBNow pins that the first page
// of a session (no cursor yet) lets Postgres's own now() serve as the
// anchor — captured back via the rank_as_of column so it can be threaded
// into the cursor for later pages (see rankAsOf in GetKnowledgeHomeItems)
// — rather than binding any value the Go layer would have had to conjure
// itself.
func TestGetKnowledgeHomeItems_FirstPageAnchorsOnDBNow(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, _, _, err := repo.GetKnowledgeHomeItems(context.Background(), uuid.New(), "", 10, nil)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL

	assert.Contains(t, sql, "now() AS rank_as_of",
		"the first page must capture the exact now() Postgres used, so it can be replayed unchanged on later pages")
}
