package sovereign_db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertTodayDigest_PreservesTopTagsFromPayload pins the contract
// that top_tags_json comes from the payload, never a hardcoded literal.
func TestUpsertTodayDigest_PreservesTopTagsFromPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"digest_date": "2026-05-04",
		"new_articles": 0,
		"summarized_articles": 0,
		"unsummarized_articles": 0,
		"top_tags": ["rust", "event-sourcing"],
		"updated_at": "2026-05-04T03:00:00Z",
		"weekly_recap_available": true,
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
		"updated_at": "2026-05-04T03:00:00Z",
		"last_event_seq": 1202
	}`)
	require.NoError(t, repo.UpsertTodayDigest(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

	assert.NotContains(t, sql, `CASE WHEN EXCLUDED.top_tags_json != '[]'::jsonb`,
		"merge-safe rule violated: SQL contains forbidden CASE pattern — replace with COALESCE/NULLIF")
	assert.Contains(t, sql, `COALESCE(NULLIF(EXCLUDED.top_tags_json, '[]'::jsonb), today_digest_view.top_tags_json)`,
		"merge-safe rule requires canonical COALESCE/NULLIF expression")
	assert.NotContains(t, sql, "pulse_refs_json")
	assert.NotContains(t, sql, "evening_pulse_available")
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
				"updated_at": "2026-05-04T03:00:00Z",
				"weekly_recap_available": true,
				"last_event_seq": 999
			}`,
			check: func(t *testing.T, m todayDigestMutation) {
				assert.Equal(t, "2026-05-04", m.DigestDate)
				assert.Equal(t, int64(999), m.LastEventSeq)
				assert.JSONEq(t, `["go","database"]`, m.TopTagsJSON)
				assert.True(t, m.WeeklyRecapAvailable)
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
				assert.Len(t, args, 9)
			}
		})
	}
}
