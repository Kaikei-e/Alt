package sovereign_db

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertRecallCandidate_PreservesReasonTypeAndDescription(t *testing.T) {
	// Capture the Exec call to inspect the reason_json argument.
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-1111-1111-111111111111",
		"item_key": "article:test-recall",
		"recall_score": 0.35,
		"reasons": [
			{"type": "opened_before_but_not_revisited", "description": "Opened 3 days ago, not revisited since", "source_item_key": ""},
			{"type": "related_to_recent_search", "description": "Related to your search for \"rust async\" (2 hours ago)"}
		],
		"next_suggest_at": "2026-03-26T00:00:00Z",
		"first_eligible_at": "2026-03-26T00:00:00Z",
		"updated_at": "2026-03-26T00:00:00Z",
		"projection_version": 1
	}`)

	err := repo.UpsertRecallCandidate(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1, "expected one Exec call")

	// reason_json is the 4th argument ($4) in the INSERT query
	reasonJSONStr, ok := mock.execCalls[0].Args[3].(string)
	require.True(t, ok, "reason_json arg should be a string")

	var reasons []struct {
		Type          string `json:"type"`
		Description   string `json:"description"`
		SourceItemKey string `json:"source_item_key,omitempty"`
	}
	err = json.Unmarshal([]byte(reasonJSONStr), &reasons)
	require.NoError(t, err)
	require.Len(t, reasons, 2)

	assert.Equal(t, "opened_before_but_not_revisited", reasons[0].Type,
		"reason type must be preserved through marshal/unmarshal round-trip")
	assert.Equal(t, "Opened 3 days ago, not revisited since", reasons[0].Description,
		"reason description must be preserved")

	assert.Equal(t, "related_to_recent_search", reasons[1].Type)
	assert.Contains(t, reasons[1].Description, "rust async")
}

// TestUpsertKnowledgeHomeItem_UsesMergeSafeSQL is the structural guard
// for the merge-safe-upsert invariant (see memory feedback_merge_safe_upsert.md
// + .claude/rules/knowledge-home.md). The UPSERT MUST NOT use SQL
// CASE expressions of the shape "WHEN EXCLUDED.x is non-empty THEN
// EXCLUDED.x ELSE <table>.x" — that smuggles business judgement into
// SQL. Instead:
//
//   - string fields (title, summary_excerpt, url) use
//     COALESCE(NULLIF(EXCLUDED.x, <EMPTY>), <table>.x);
//   - the jsonb tags array uses
//     COALESCE(NULLIF(EXCLUDED.tags_json, <EMPTY-ARRAY>::jsonb), <table>.tags_json);
//   - summary_state uses GREATEST(<table>.summary_state, EXCLUDED.summary_state)
//     (lexicographic monotonic latch: empty < missing < pending < ready).
//     score's merge follows the same floor idea for its 'max' branch, but
//     is selected via the score_op parameter rather than applied
//     unconditionally — see TestUpsertKnowledgeHomeItem_ScoreMergeHonorsScoreOp.
//
// Inline placeholders <EMPTY> and <EMPTY-ARRAY> stand for the SQL empty
// string literal and the JSONB empty array literal respectively — the
// raw two-single-quote forms are avoided in this comment because Go
// 1.26's gofmt normalises consecutive single quotes inside doc comments
// into Unicode close-quotes, which would corrupt the SQL examples.
//
// The why_json merge intentionally keeps its `SELECT DISTINCT ON … source_rank`
// expression — that is a deterministic merge over array members keyed
// by `code`, not a business-logic CASE. The UPSERT body here only forbids
// the latter.
func TestUpsertKnowledgeHomeItem_UsesMergeSafeSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	payload := []byte(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"tenant_id": "22222222-2222-4222-8222-222222222222",
		"item_key": "article:33333333-3333-4333-8333-333333333333",
		"item_type": "article",
		"primary_ref_id": "33333333-3333-4333-8333-333333333333",
		"title": "t",
		"summary_excerpt": "x",
		"tags": ["go", "event-sourcing"],
		"why_reasons": [{"code": "new_unread", "reason": "."}],
		"score": 0.5,
		"score_op": "max",
		"freshness_at": "` + now + `",
		"generated_at": "` + now + `",
		"updated_at": "` + now + `",
		"projection_version": 7,
		"summary_state": "pending",
		"url": "https://example.com/x"
	}`)
	_ = uuid.New // keep uuid import to align with the rest of the test file

	require.NoError(t, repo.UpsertKnowledgeHomeItem(context.Background(), json.RawMessage(payload)))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	// Forbidden CASE patterns — these are the business-logic constructs
	// the refactor exists to remove. If any reappear, the test fails
	// and points at the merge-safe rule.
	for _, banned := range []string{
		`CASE WHEN EXCLUDED.title != ''`,
		`CASE WHEN EXCLUDED.summary_excerpt != ''`,
		`CASE WHEN EXCLUDED.tags_json != '[]'::jsonb`,
		`CASE WHEN EXCLUDED.summary_state = 'ready'`,
		`CASE WHEN EXCLUDED.url != ''`,
	} {
		assert.NotContains(t, sql, banned,
			"merge-safe rule violated: SQL contains forbidden CASE pattern %q — replace with COALESCE/NULLIF/GREATEST", banned)
	}

	// Required canonical merge expressions for each of the 5 fields.
	for _, required := range []string{
		`COALESCE(NULLIF(EXCLUDED.title, ''), knowledge_home_items.title)`,
		`COALESCE(NULLIF(EXCLUDED.summary_excerpt, ''), knowledge_home_items.summary_excerpt)`,
		`COALESCE(NULLIF(EXCLUDED.tags_json, '[]'::jsonb), knowledge_home_items.tags_json)`,
		`GREATEST(knowledge_home_items.summary_state, EXCLUDED.summary_state)`,
		`COALESCE(NULLIF(EXCLUDED.url, ''), knowledge_home_items.url)`,
	} {
		assert.True(t, strings.Contains(sql, required),
			"merge-safe rule requires canonical expression %q — actual SQL omits it", required)
	}
}

// scoreOpUpsertPayload builds a minimal UpsertKnowledgeHomeItem payload with
// the given score_op. A nil scoreOp omits the JSON key entirely (simulating
// a caller unaware of the field, like the ApplyProjectionMutation RPC
// producer); a non-nil scoreOp — including "" — emits the key explicitly.
func scoreOpUpsertPayload(t *testing.T, scoreOp *string) []byte {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	fields := map[string]any{
		"user_id":            "11111111-1111-4111-8111-111111111111",
		"tenant_id":          "22222222-2222-4222-8222-222222222222",
		"item_key":           "article:33333333-3333-4333-8333-333333333333",
		"item_type":          "article",
		"score":              0.1,
		"generated_at":       now,
		"updated_at":         now,
		"projection_version": 1,
	}
	if scoreOp != nil {
		fields["score_op"] = *scoreOp
	}
	raw, err := json.Marshal(fields)
	require.NoError(t, err)
	return raw
}

// scoreCasePattern extracts the two score_op-gated WHEN/THEN branches from
// the UPSERT's `score = CASE ... END` clause: WHEN $23 = '<op1>' THEN
// <expr1>, WHEN $23 = '<op2>' THEN <expr2>, ELSE <expr3>.
var scoreCasePattern = regexp.MustCompile(
	`(?s)score = CASE\s*WHEN \$23 = '([a-z]*)' THEN (EXCLUDED\.score)\s*` +
		`WHEN \$23 = '([a-z]*)' THEN (GREATEST\(EXCLUDED\.score, knowledge_home_items\.score\))\s*` +
		`ELSE (knowledge_home_items\.score)\s*END,`)

// TestUpsertKnowledgeHomeItem_ScoreMergeHonorsScoreOp pins the fix for the
// unreachable-suppression defect: a blanket `score = GREATEST(EXCLUDED.score,
// knowledge_home_items.score)` can only ever ratchet a score up, so
// HomeItemOpened's suppressed 0.1 score could never overwrite any higher
// stored score. The merge must instead branch on the payload's score_op:
// "set" overwrites unconditionally (including downward), "max" keeps the
// floor semantics the other folds rely on.
//
// A prior version of this test only checked for the bare presence of the
// strings "GREATEST(...)" / "EXCLUDED.score" anywhere in the SQL and that
// args[22] == "set" — none of which ties the literal "set" to the overwrite
// branch specifically. Mutation-proven: collapsing the CASE back to
// `WHEN $23 = 'never' THEN EXCLUDED.score ELSE GREATEST(...) END`
// (score_op='set' can then never match, so every write silently falls
// through to the floor-only branch — the exact pre-fix bug) left all of
// those assertions passing. This version extracts the WHEN/THEN pairs with
// scoreCasePattern and asserts which score_op literal is wired to which
// branch, so that regression is caught. See mutationProofs for the
// captured failure output.
func TestUpsertKnowledgeHomeItem_ScoreMergeHonorsScoreOp(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	setOp := "set"
	require.NoError(t, repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &setOp)))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	m := scoreCasePattern.FindStringSubmatch(sql)
	require.NotNil(t, m, "expected a score CASE with two $23-gated WHEN branches (set -> EXCLUDED.score, "+
		"max -> GREATEST(...)); got SQL:\n%s", sql)
	assert.Equal(t, "set", m[1], "the branch that overwrites with EXCLUDED.score unconditionally must be gated on score_op == 'set'")
	assert.Equal(t, "max", m[3], "the branch that applies floor semantics (GREATEST) must be gated on score_op == 'max'")

	require.Len(t, args, 23, "score_op must be bound as its own parameter")
	assert.Equal(t, "set", args[22], "score_op must be bound from the payload, not hardcoded")
}

// TestUpsertKnowledgeHomeItem_ScoreOpMaxBindsMaxLiteral is the "max" half of
// the args-binding check above: proves the bound parameter reflects
// whichever score_op the payload actually carried, not a value fixed by
// whichever branch happened to run first.
func TestUpsertKnowledgeHomeItem_ScoreOpMaxBindsMaxLiteral(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	maxOp := "max"
	require.NoError(t, repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &maxOp)))
	require.Len(t, mock.execCalls, 1)
	args := mock.execCalls[0].Args
	require.Len(t, args, 23)
	assert.Equal(t, "max", args[22])
}

// TestUpsertKnowledgeHomeItem_EmptyScoreOpIsAcceptedAsExplicitNoop pins that
// score_op present-but-empty (what folds that never touch score, e.g. the
// supersede folds, emit — see homeItemWrite in knowledge_home_projector) is
// a legitimate "leave score untouched" and must not be rejected the same
// way an absent key is.
func TestUpsertKnowledgeHomeItem_EmptyScoreOpIsAcceptedAsExplicitNoop(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	emptyOp := ""
	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &emptyOp))
	require.NoError(t, err, "an explicitly empty score_op must be accepted (it is how no-touch folds signal intent)")
	require.Len(t, mock.execCalls, 1)
	assert.Equal(t, "", mock.execCalls[0].Args[22])
}

// TestUpsertKnowledgeHomeItem_RejectsMissingScoreOp pins the fix for the
// silent-fallback defect (Alt Rule 8): a payload whose score_op key is
// absent entirely — the exact shape the ApplyProjectionMutation RPC
// producer sends, since its wire type carries no score_op field at all —
// used to unmarshal to the Go zero value "" and take the CASE's ELSE
// branch, silently discarding the caller's intended score forever with no
// error anywhere. An absent key is now indistinguishable from neither
// "leave untouched" nor a recognized operator, so the write must be
// refused rather than guessed at.
func TestUpsertKnowledgeHomeItem_RejectsMissingScoreOp(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, nil))
	require.Error(t, err, "a payload with no score_op key at all must fail loudly, not silently discard the score")
	assert.Empty(t, mock.execCalls, "no write should reach the database with an unresolved score_op")
}

// TestUpsertKnowledgeHomeItem_RejectsUnrecognizedScoreOp guards against a
// typo'd score_op silently falling through to "leave untouched" the same
// way an absent key would — e.g. a producer that meant "set" but sent
// "sett" must be told loudly, not have its write vanish.
func TestUpsertKnowledgeHomeItem_RejectsUnrecognizedScoreOp(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	bogusOp := "increment"
	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &bogusOp))
	require.Error(t, err, "an unrecognized score_op must fail loudly, not silently discard the score")
	assert.Empty(t, mock.execCalls)
}

// TestUpsertTodayDigest_PreservesPulseRefsFromPayload pins the contract
// that pulse_refs_json comes from the payload, never a hardcoded literal.
// The earlier implementation hardcoded pulseRefsJSON := []byte("[]") which
// permanently froze today_digest_view.pulse_refs_json at empty for every
// user, masking Evening Pulse availability in the Morning Letter UI.
func TestUpsertTodayDigest_PreservesPulseRefsFromPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 0,
		"summarized_articles": 0,
		"unsummarized_articles": 0,
		"top_tags": ["rust", "event-sourcing"],
		"pulse_refs": ["cluster:42", "cluster:99"],
		"updated_at": "2026-05-04T03:00:00Z",
		"weekly_recap_available": true,
		"evening_pulse_available": true
	}`)

	require.NoError(t, repo.UpsertTodayDigest(context.Background(), payload))
	require.Len(t, mock.execCalls, 1, "expected one Exec call")

	args := mock.execCalls[0].Args
	require.GreaterOrEqual(t, len(args), 7, "expected at least 7 args in INSERT")

	topTagsStr, ok := args[5].(string)
	require.True(t, ok, "top_tags_json arg ($6) must be a string")
	assert.JSONEq(t, `["rust","event-sourcing"]`, topTagsStr,
		"top_tags_json must reflect payload.top_tags, not be hardcoded")

	pulseRefsStr, ok := args[6].(string)
	require.True(t, ok, "pulse_refs_json arg ($7) must be a string")
	assert.JSONEq(t, `["cluster:42","cluster:99"]`, pulseRefsStr,
		"pulse_refs_json must reflect payload.pulse_refs, not be hardcoded to []")
}

// TestUpsertTodayDigest_UsesMergeSafeSQL is the structural guard for the
// merge-safe-upsert invariant on today_digest_view, mirroring the
// guard for knowledge_home_items. SQL must use COALESCE/NULLIF for jsonb
// arrays, never `CASE WHEN EXCLUDED.x != '[]'::jsonb` (business judgement
// in SQL — see feedback_merge_safe_upsert.md).
func TestUpsertTodayDigest_UsesMergeSafeSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"top_tags": ["go"],
		"pulse_refs": ["cluster:1"],
		"updated_at": "2026-05-04T03:00:00Z"
	}`)
	require.NoError(t, repo.UpsertTodayDigest(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	for _, banned := range []string{
		`CASE WHEN EXCLUDED.top_tags_json != '[]'::jsonb`,
		`CASE WHEN EXCLUDED.pulse_refs_json != '[]'::jsonb`,
	} {
		assert.NotContains(t, sql, banned,
			"merge-safe rule violated: SQL contains forbidden CASE pattern %q — replace with COALESCE/NULLIF", banned)
	}

	for _, required := range []string{
		`COALESCE(NULLIF(EXCLUDED.top_tags_json, '[]'::jsonb), today_digest_view.top_tags_json)`,
		`COALESCE(NULLIF(EXCLUDED.pulse_refs_json, '[]'::jsonb), today_digest_view.pulse_refs_json)`,
	} {
		assert.True(t, strings.Contains(sql, required),
			"merge-safe rule requires canonical expression %q — actual SQL omits it", required)
	}
}

// TestUpsertTodayDigest_ReplayGuardPreventsDoubleCounting is the structural
// guard for the idempotent-upsert invariant (event-stream-consumer.md:
// projection UPSERTs must overwrite absolute values, additive merges are
// forbidden; immutable-design-guard's Merge-safe upsert principle).
// new_articles/summarized_articles/unsummarized_articles are additive
// deltas contributed per source event — an unconditional col = col + delta
// double-counts on an at-least-once RPC resend or a full reprojection
// replay of the same event. The fix guards the UPDATE with
// EXCLUDED.updated_at > today_digest_view.updated_at: since updated_at is
// always the source event's OccurredAt (never wall-clock), replaying the
// identical event becomes a no-op.
func TestUpsertTodayDigest_ReplayGuardPreventsDoubleCounting(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 1,
		"updated_at": "2026-05-04T03:00:00Z"
	}`)
	require.NoError(t, repo.UpsertTodayDigest(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	assert.Contains(t, sql, "WHERE EXCLUDED.updated_at > today_digest_view.updated_at",
		"additive counters must be guarded by a strictly-newer-event check")
}

// TestDismissKnowledgeHomeItem_RequiresDismissedAtInPayload pins the fix for
// the business-fact time.Now() bug: dismissed_at must come from the event
// payload. Fabricating it with time.Now() would make replaying the same
// DismissedHomeItem event produce a different dismissed_at each time,
// breaking reproject-safety (immutable-design-guard: Event-time purity).
func TestDismissKnowledgeHomeItem_RequiresDismissedAtInPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"projection_version": 1
	}`)

	err := repo.DismissKnowledgeHomeItem(context.Background(), payload)
	require.Error(t, err, "missing dismissed_at must error loudly, not fabricate time.Now()")
	assert.Empty(t, mock.execCalls, "no UPDATE should be issued when dismissed_at is missing")
}

// TestDismissKnowledgeHomeItem_ReplayIsDeterministic asserts that
// reprojecting the same DismissedHomeItem event twice writes the identical
// dismissed_at both times.
func TestDismissKnowledgeHomeItem_ReplayIsDeterministic(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"projection_version": 1,
		"dismissed_at": "2026-05-04T03:00:00Z"
	}`)

	require.NoError(t, repo.DismissKnowledgeHomeItem(context.Background(), payload))
	require.NoError(t, repo.DismissKnowledgeHomeItem(context.Background(), payload))
	require.Len(t, mock.execCalls, 2)

	first := mock.execCalls[0].Args[0]
	second := mock.execCalls[1].Args[0]
	assert.Equal(t, first, second,
		"reprojecting the same event twice must write the identical dismissed_at — no wall-clock drift")
}

// TestSnoozeRecallCandidate_RequiresOccurredAtInPayload pins the fix that
// replaces SQL now() with a required occurred_at payload field.
func TestSnoozeRecallCandidate_RequiresOccurredAtInPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"until": "2026-05-05T00:00:00Z"
	}`)

	err := repo.SnoozeRecallCandidate(context.Background(), payload)
	require.Error(t, err, "missing occurred_at must error loudly, not fall back to SQL now()")
	assert.Empty(t, mock.execCalls)
}

// TestSnoozeRecallCandidate_WritesOccurredAtNotNow verifies the UPDATE no
// longer calls SQL now() and instead binds the payload's occurred_at.
func TestSnoozeRecallCandidate_WritesOccurredAtNotNow(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"until": "2026-05-05T00:00:00Z",
		"occurred_at": "2026-05-04T03:00:00Z"
	}`)

	require.NoError(t, repo.SnoozeRecallCandidate(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL
	assert.NotContains(t, sql, "now()", "updated_at must come from the occurred_at parameter, not SQL now()")
}

// TestDismissRecallCandidate_RequiresOccurredAtInPayload mirrors the Snooze
// fix for DismissRecallCandidate.
func TestDismissRecallCandidate_RequiresOccurredAtInPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test"
	}`)

	err := repo.DismissRecallCandidate(context.Background(), payload)
	require.Error(t, err, "missing occurred_at must error loudly, not fall back to SQL now()")
	assert.Empty(t, mock.execCalls)
}

func TestDismissRecallCandidate_WritesOccurredAtNotNow(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"occurred_at": "2026-05-04T03:00:00Z"
	}`)

	require.NoError(t, repo.DismissRecallCandidate(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL
	assert.NotContains(t, sql, "now()", "dismissed_at/updated_at must come from the occurred_at parameter, not SQL now()")
}
