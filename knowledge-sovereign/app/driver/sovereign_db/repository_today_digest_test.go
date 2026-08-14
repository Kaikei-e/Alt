package sovereign_db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertTodayDigest_RequiresLastEventSeq pins the producer-wiring contract
// for the replay guard. The guard can only mean "already folded" if it is
// keyed on the log's own order; a payload that omits last_event_seq comes from
// a producer that does not know about the field and must be told loudly rather
// than silently fall back to a wall-clock comparison (Alt Rule 8: no silent
// fallback for an unwired write path — same shape as score_op on
// UpsertKnowledgeHomeItem).
func TestUpsertTodayDigest_RequiresLastEventSeq(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 1,
		"updated_at": "2026-05-04T03:00:00Z"
	}`)

	err := repo.UpsertTodayDigest(context.Background(), payload)
	require.Error(t, err, "a digest write with no last_event_seq must fail loudly, not fall back to the wall clock")
	assert.Contains(t, err.Error(), "last_event_seq")
	assert.Empty(t, mock.execCalls, "nothing may reach the database once the payload is rejected")
}

// TestUpsertTodayDigest_RejectsNonPositiveLastEventSeq guards the other half of
// the same contract. knowledge_events.event_seq is BIGSERIAL, so it is always
// >= 1; a zero (the Go/JSON zero value, and the column default carried by rows
// written before the column existed) would tie or lose against every stored
// value and quietly strand the row forever.
func TestUpsertTodayDigest_RejectsNonPositiveLastEventSeq(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 1,
		"updated_at": "2026-05-04T03:00:00Z",
		"last_event_seq": 0
	}`)

	err := repo.UpsertTodayDigest(context.Background(), payload)
	require.Error(t, err, "event_seq 0 never identifies a real event and must be rejected")
	assert.Contains(t, err.Error(), "last_event_seq")
	assert.Empty(t, mock.execCalls)
}

// TestUpsertTodayDigest_ReplayGuardOrdersByEventSeqNotWallClock is the
// structural guard for finding 056.
//
// updated_at is the source event's occurred_at, and occurred_at is stamped
// with time.Now() by whichever producer emitted the event before the append
// RPC — six-plus independent producers, six-plus independent clocks. The fold
// order is event_seq. When the two disagree for the same user and day, an
// `EXCLUDED.updated_at > today_digest_view.updated_at` guard throws away the
// whole DO UPDATE of the older-looking event — its counter deltas included —
// and TodayBar stays permanently one short. event_seq is monotonic in exactly
// the order the projector folds, so it is the only discriminator that
// distinguishes "already folded" from "another machine's clock is ahead".
func TestUpsertTodayDigest_ReplayGuardOrdersByEventSeqNotWallClock(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	require.NoError(t, repo.UpsertTodayDigest(context.Background(), json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 1,
		"updated_at": "2026-05-04T03:00:00Z",
		"last_event_seq": 4711
	}`)))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	assert.Contains(t, sql, "WHERE EXCLUDED.last_event_seq > today_digest_view.last_event_seq",
		"the additive counters must be gated on the log's own order, not on a producer's clock")
	assert.NotContains(t, sql, "EXCLUDED.updated_at > today_digest_view.updated_at",
		"a wall-clock guard silently drops the increments of any event whose producer clock lags")
	assert.Contains(t, sql, "last_event_seq = EXCLUDED.last_event_seq",
		"the high-water mark must advance, otherwise every later event re-passes the guard")

	require.Len(t, mock.execCalls[0].Args, 11, "last_event_seq must be bound as the 11th parameter")
	assert.EqualValues(t, 4711, mock.execCalls[0].Args[10],
		"last_event_seq must reach the statement as the payload's event_seq")
}

// TestUpsertTodayDigest_FirstWriteOfDayFloorsNegativeUnsummarized is the
// structural guard for finding 057.
//
// unsummarized_articles is a signed delta: SummaryVersionCreated contributes
// -1. GREATEST(0, ...) used to live only in the ON CONFLICT branch, so a
// midnight batch summarising yesterday's articles — where the -1 is the first
// digest write of the new day — took the plain INSERT path and stored -1.
// The column has no CHECK and GetTodayDigest returns it verbatim, so that day
// reads one short for good.
//
// The floor therefore belongs on the INSERT operand, and the ON CONFLICT
// branch must then read the delta from the bare parameter rather than
// EXCLUDED: EXCLUDED is the row *after* the VALUES expressions are evaluated,
// so it would carry the floored 0 and the decrement would silently become a
// no-op.
func TestUpsertTodayDigest_FirstWriteOfDayFloorsNegativeUnsummarized(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	require.NoError(t, repo.UpsertTodayDigest(context.Background(), json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-05",
		"summarized_articles": 1,
		"unsummarized_articles": -1,
		"updated_at": "2026-05-05T00:03:00Z",
		"last_event_seq": 90210
	}`)))
	require.Len(t, mock.execCalls, 1)
	call := mock.execCalls[0]

	assert.Contains(t, call.SQL, "GREATEST(0, $5)",
		"the INSERT operand must be floored, or the first write of the day stores a negative count")

	assert.Contains(t, call.SQL, "unsummarized_articles = GREATEST(0, today_digest_view.unsummarized_articles + $5)",
		"the conflict branch must add the signed delta from the bare parameter")
	assert.NotContains(t, call.SQL, "today_digest_view.unsummarized_articles + EXCLUDED.unsummarized_articles",
		"EXCLUDED carries the floored VALUES expression, so a -1 decrement would become +0")

	require.GreaterOrEqual(t, len(call.Args), 5)
	assert.EqualValues(t, -1, call.Args[4],
		"the signed delta must still be bound raw — flooring in Go would break the decrement of an existing row")

	// The other two counters are only ever 0 or +1, so they stay unfloored;
	// pin that so nobody "fixes" them into a floor that hides a real bug.
	assert.False(t, strings.Contains(call.SQL, "GREATEST(0, $3)") || strings.Contains(call.SQL, "GREATEST(0, $4)"),
		"new_articles/summarized_articles are never negative and must not be floored")
}
