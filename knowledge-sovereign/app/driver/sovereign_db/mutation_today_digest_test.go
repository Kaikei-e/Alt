package sovereign_db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertTodayDigest_PreservesPulseRefsFromPayload pins the contract
// that pulse_refs_json comes from the payload, never a hardcoded literal.
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
		"evening_pulse_available": true,
		"last_event_seq": 1201
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
// arrays, never CASE expressions.
func TestUpsertTodayDigest_UsesMergeSafeSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"top_tags": ["go"],
		"pulse_refs": ["cluster:1"],
		"updated_at": "2026-05-04T03:00:00Z",
		"last_event_seq": 1202
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
// guard for the idempotent-upsert invariant.
func TestUpsertTodayDigest_ReplayGuardPreventsDoubleCounting(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 1,
		"updated_at": "2026-05-04T03:00:00Z",
		"last_event_seq": 1203
	}`)
	require.NoError(t, repo.UpsertTodayDigest(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	assert.Contains(t, sql, "WHERE EXCLUDED.last_event_seq > today_digest_view.last_event_seq",
		"additive counters must be guarded by a strictly-newer-event check")
}

func TestParseTodayDigestMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m todayDigestMutation)
	}{
		{
			name: "valid full payload",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"digest_date": "2026-05-04",
				"new_articles": 2,
				"summarized_articles": 1,
				"unsummarized_articles": 1,
				"top_tags": ["go", "database"],
				"pulse_refs": ["ref-1"],
				"updated_at": "2026-05-04T03:00:00Z",
				"weekly_recap_available": true,
				"evening_pulse_available": false,
				"last_event_seq": 999
			}`,
			check: func(t *testing.T, m todayDigestMutation) {
				assert.Equal(t, "2026-05-04", m.DigestDate)
				assert.Equal(t, int64(999), m.LastEventSeq)
				assert.JSONEq(t, `["go","database"]`, m.TopTagsJSON)
				assert.JSONEq(t, `["ref-1"]`, m.PulseRefsJSON)
				assert.True(t, m.WeeklyRecapAvailable)
				assert.False(t, m.EveningPulseAvailable)
				assert.Equal(t, time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC), m.UpdatedAt)
			},
		},
		{
			name: "valid minimal payload defaults empty slices",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"digest_date": "2026-05-04",
				"last_event_seq": 1
			}`,
			check: func(t *testing.T, m todayDigestMutation) {
				assert.Equal(t, "[]", m.TopTagsJSON)
				assert.Equal(t, "[]", m.PulseRefsJSON)
				assert.Equal(t, int64(1), m.LastEventSeq)
			},
		},
		{
			name:       "missing last_event_seq",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "digest_date": "2026-05-04"}`,
			wantErrMsg: "last_event_seq is required",
		},
		{
			name:       "zero last_event_seq",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "digest_date": "2026-05-04", "last_event_seq": 0}`,
			wantErrMsg: "last_event_seq must be a positive event_seq, got 0",
		},
		{
			name:       "negative last_event_seq",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "digest_date": "2026-05-04", "last_event_seq": -5}`,
			wantErrMsg: "last_event_seq must be a positive event_seq, got -5",
		},
		{
			name:       "invalid json",
			payload:    `{bad-json`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseTodayDigestMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				args := buildUpsertTodayDigestArgs(m)
				assert.Len(t, args, 11)
			}
		})
	}
}
