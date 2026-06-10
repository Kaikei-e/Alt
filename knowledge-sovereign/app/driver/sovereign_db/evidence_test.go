package sovereign_db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The accumulator's ring-trim / seq-guard / pin-coalesce semantics live in the
// ON CONFLICT SQL and are exercised against real Postgres in the migration's
// integration check. These unit tests pin the Go-side contract the projector
// relies on: the statement shape (seq guard present), the fact vs pin arg
// routing, and the fail-closed guards.

func TestUpsertKnowledgeLoopEvidence_FactStyle_AppendsRingWithSeqGuard(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	occ := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	err := repo.UpsertKnowledgeLoopEvidence(context.Background(), KnowledgeLoopEvidenceWrite{
		UserID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		ScopeKind:  "article",
		ScopeRef:   "art-A",
		SignalKind: "summary_supersede",
		NewFact:    &KnowledgeLoopEvidenceFact{OccurredAt: occ, EventSeq: 300},
		OccurredAt: occ,
		EventSeq:   300,
	})
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	assert.Contains(t, sql, "ON CONFLICT (user_id, scope_kind, scope_ref, signal_kind)")
	assert.Contains(t, sql, "WHERE knowledge_loop_evidence.last_event_seq < EXCLUDED.last_event_seq",
		"the seq guard is what makes replay a no-op — it must be present")
	assert.Contains(t, sql, "LIMIT 32", "ring must be bounded")

	args := mock.execCalls[0].Args
	// $6 facts: a one-element array carrying the new fact.
	factsArg, ok := args[5].(string)
	require.True(t, ok, "facts arg ($6) must be a JSON string")
	var facts []KnowledgeLoopEvidenceFact
	require.NoError(t, json.Unmarshal([]byte(factsArg), &facts))
	require.Len(t, facts, 1)
	assert.Equal(t, int64(300), facts[0].EventSeq)
	// $7 facts_total delta = 1 for a fact-style write.
	assert.Equal(t, 1, args[6])
	// $8 pinned_text is nil for a fact-style write (so COALESCE keeps any pin).
	assert.Nil(t, args[7])
	// $11 last_event_seq.
	assert.Equal(t, int64(300), args[10])
}

func TestUpsertKnowledgeLoopEvidence_PinStyle_SetsPinAndZeroDelta(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	occ := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	err := repo.UpsertKnowledgeLoopEvidence(context.Background(), KnowledgeLoopEvidenceWrite{
		UserID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		ScopeKind:  "article",
		ScopeRef:   "art-A",
		SignalKind: "url_pin",
		PinnedText: "https://example.com/a",
		OccurredAt: occ,
		EventSeq:   12,
	})
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	args := mock.execCalls[0].Args
	// $6 facts is the empty array so the ring is untouched on conflict.
	assert.Equal(t, "[]", args[5])
	// $7 facts_total delta = 0 for a pin-style write.
	assert.Equal(t, 0, args[6])
	// $8 pinned_text carries the value.
	require.NotNil(t, args[7])
	pinned, ok := args[7].(*string)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/a", *pinned)
}

func TestUpsertKnowledgeLoopEvidence_PinnedPayloadRoundTrips(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	err := repo.UpsertKnowledgeLoopEvidence(context.Background(), KnowledgeLoopEvidenceWrite{
		UserID:        uuid.New(),
		TenantID:      uuid.New(),
		ScopeKind:     "article",
		ScopeRef:      "art-A",
		SignalKind:    "tag_set_current",
		PinnedPayload: []byte(`{"tags":["go","rust"]}`),
		OccurredAt:    time.Now().UTC(),
		EventSeq:      7,
	})
	require.NoError(t, err)
	args := mock.execCalls[0].Args
	// $9 pinned_payload passed through as a JSON string.
	assert.Equal(t, `{"tags":["go","rust"]}`, args[8])
}

func TestUpsertKnowledgeLoopEvidence_RejectsNilUserAndEmptyScope(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	err := repo.UpsertKnowledgeLoopEvidence(context.Background(), KnowledgeLoopEvidenceWrite{
		UserID: uuid.Nil, TenantID: uuid.New(), ScopeKind: "article", ScopeRef: "x", SignalKind: "url_pin",
	})
	require.Error(t, err, "nil user_id is a programmer error (F-001), not a silent no-op")

	err = repo.UpsertKnowledgeLoopEvidence(context.Background(), KnowledgeLoopEvidenceWrite{
		UserID: uuid.New(), TenantID: uuid.New(), ScopeKind: "", ScopeRef: "x", SignalKind: "url_pin",
	})
	require.Error(t, err, "empty scope_kind must be rejected")

	require.Empty(t, mock.execCalls, "no SQL should be issued for invalid writes")
}

func TestGetKnowledgeLoopEvidenceForScopes_GuardsAndTupleMatch(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetKnowledgeLoopEvidenceForScopes(context.Background(), uuid.Nil, []KnowledgeLoopEvidenceScope{{ScopeKind: "entry", ScopeRef: "e"}})
	require.Error(t, err, "nil user_id must be rejected")

	out, err := repo.GetKnowledgeLoopEvidenceForScopes(context.Background(), uuid.New(), nil)
	require.NoError(t, err)
	require.Nil(t, out, "no scopes → no query, no rows")
}

func TestTruncateKnowledgeLoopEvidence_IssuesTruncate(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}
	require.NoError(t, repo.TruncateKnowledgeLoopEvidence(context.Background()))
	require.Len(t, mock.execCalls, 1)
	assert.True(t, strings.Contains(mock.execCalls[0].SQL, "TRUNCATE TABLE knowledge_loop_evidence"))
}
