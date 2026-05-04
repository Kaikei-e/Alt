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
//     (lexicographic monotonic latch: empty < missing < pending < ready;
//     same pattern used for score = GREATEST(...) already in this file).
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
