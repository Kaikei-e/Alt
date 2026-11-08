package sovereign_db

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertRecallCandidate_PreservesReasonTypeAndDescription(t *testing.T) {
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

func TestUpsertRecallCandidate_DeduplicatesIdenticalReasons(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-1111-1111-111111111111",
		"item_key": "article:test-recall-dup",
		"recall_score": 0.5,
		"reasons": [
			{"type": "opened_before_but_not_revisited", "description": "Opened 6 days ago, not revisited since"},
			{"type": "opened_before_but_not_revisited", "description": "Opened 6 days ago, not revisited since"},
			{"type": "opened_before_but_not_revisited", "description": "Opened 6 days ago, not revisited since"}
		],
		"next_suggest_at": "2026-03-26T00:00:00Z",
		"first_eligible_at": "2026-03-26T00:00:00Z",
		"updated_at": "2026-03-26T00:00:00Z",
		"projection_version": 1
	}`)

	require.NoError(t, repo.UpsertRecallCandidate(context.Background(), payload))
	require.Len(t, mock.execCalls, 1)

	reasonJSONStr, ok := mock.execCalls[0].Args[3].(string)
	require.True(t, ok)

	var reasons []RecallReason
	require.NoError(t, json.Unmarshal([]byte(reasonJSONStr), &reasons))
	assert.Len(t, reasons, 1, "identical reasons must be deduplicated")
}

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

func TestParseUpsertRecallCandidateMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m upsertRecallCandidateMutation)
	}{
		{
			name: "valid payload",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:rec",
				"recall_score": 0.42,
				"reasons": [{"type": "t1", "description": "d1"}],
				"projection_version": 2
			}`,
			check: func(t *testing.T, m upsertRecallCandidateMutation) {
				assert.Equal(t, "article:rec", m.ItemKey)
				assert.Equal(t, 0.42, m.RecallScore)
				assert.JSONEq(t, `[{"type":"t1","description":"d1"}]`, m.ReasonJSON)
				assert.Equal(t, 2, m.ProjectionVersion)
			},
		},
		{
			name:       "invalid json",
			payload:    `{invalid`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseUpsertRecallCandidateMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				args := buildUpsertRecallCandidateArgs(m)
				assert.Len(t, args, 8)
			}
		})
	}
}

func TestParseSnoozeRecallCandidateMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m snoozeRecallCandidateMutation)
	}{
		{
			name: "valid snooze",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:snooze",
				"until": "2026-05-10T00:00:00Z",
				"occurred_at": "2026-05-04T03:00:00Z"
			}`,
			check: func(t *testing.T, m snoozeRecallCandidateMutation) {
				assert.Equal(t, uuid.MustParse("11111111-1111-4111-8111-111111111111"), m.UserID)
				assert.Equal(t, "article:snooze", m.ItemKey)
				assert.Equal(t, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), m.Until)
				assert.Equal(t, time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC), m.OccurredAt)
			},
		},
		{
			name:       "missing occurred_at",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "k", "until": "2026-05-10T00:00:00Z"}`,
			wantErrMsg: "occurred_at is required",
		},
		{
			name:       "invalid user_id",
			payload:    `{"user_id": "bad-id", "item_key": "k", "until": "2026-05-10T00:00:00Z", "occurred_at": "2026-05-04T03:00:00Z"}`,
			wantErrMsg: "parse user_id",
		},
		{
			name:       "invalid until",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "k", "until": "bad-time", "occurred_at": "2026-05-04T03:00:00Z"}`,
			wantErrMsg: "parse until",
		},
		{
			name:       "invalid occurred_at",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "k", "until": "2026-05-10T00:00:00Z", "occurred_at": "bad-time"}`,
			wantErrMsg: "parse occurred_at",
		},
		{
			name:       "invalid json",
			payload:    `{bad`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseSnoozeRecallCandidateMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				args := buildSnoozeRecallCandidateArgs(m)
				assert.Len(t, args, 4)
			}
		})
	}
}

func TestParseDismissRecallCandidateMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m dismissRecallCandidateMutation)
	}{
		{
			name: "valid dismiss",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:dismiss",
				"occurred_at": "2026-05-04T03:00:00Z"
			}`,
			check: func(t *testing.T, m dismissRecallCandidateMutation) {
				assert.Equal(t, uuid.MustParse("11111111-1111-4111-8111-111111111111"), m.UserID)
				assert.Equal(t, "article:dismiss", m.ItemKey)
				assert.Equal(t, time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC), m.OccurredAt)
			},
		},
		{
			name:       "missing occurred_at",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "k"}`,
			wantErrMsg: "occurred_at is required",
		},
		{
			name:       "invalid user_id",
			payload:    `{"user_id": "bad-id", "item_key": "k", "occurred_at": "2026-05-04T03:00:00Z"}`,
			wantErrMsg: "parse user_id",
		},
		{
			name:       "invalid occurred_at",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "k", "occurred_at": "bad-date"}`,
			wantErrMsg: "parse occurred_at",
		},
		{
			name:       "invalid json",
			payload:    `{bad`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseDismissRecallCandidateMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				args := buildDismissRecallCandidateArgs(m)
				assert.Len(t, args, 3)
			}
		})
	}
}

func TestRecallCandidateMutations_ProjectionVersionCutover(t *testing.T) {
	ctx := context.Background()
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	tenantID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	itemKey := "article:cutover-recall"

	t.Run("structural SQL uses incoming/active projection_version instead of existing row version", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		upsertPayload, err := json.Marshal(map[string]any{
			"user_id":            userID.String(),
			"tenant_id":          tenantID.String(),
			"item_key":           itemKey,
			"recall_score":       0.75,
			"updated_at":         fixedTestTimeRFC3339,
			"projection_version": 2,
		})
		require.NoError(t, err)
		require.NoError(t, repo.UpsertRecallCandidate(ctx, upsertPayload))

		snoozePayload, err := json.Marshal(map[string]any{
			"user_id":     userID.String(),
			"tenant_id":   tenantID.String(),
			"item_key":    itemKey,
			"until":       "2026-05-05T03:00:00Z",
			"occurred_at": fixedTestTimeRFC3339,
		})
		require.NoError(t, err)
		require.NoError(t, repo.SnoozeRecallCandidate(ctx, snoozePayload))

		dismissPayload, err := json.Marshal(map[string]any{
			"user_id":     userID.String(),
			"tenant_id":   tenantID.String(),
			"item_key":    itemKey,
			"occurred_at": fixedTestTimeRFC3339,
		})
		require.NoError(t, err)
		require.NoError(t, repo.DismissRecallCandidate(ctx, dismissPayload))

		require.Len(t, mock.execCalls, 3)

		upsertSQL := mock.execCalls[0].SQL
		assert.Contains(t, upsertSQL, "khi.projection_version = $8")
		assert.Contains(t, upsertSQL, "khi.projection_version = EXCLUDED.projection_version")
		assert.NotContains(t, upsertSQL, "recall_candidate_view.projection_version")

		snoozeSQL := mock.execCalls[1].SQL
		assert.Contains(t, snoozeSQL, "khi.projection_version = "+activeProjectionVersionSQL)
		assert.NotContains(t, snoozeSQL, "recall_candidate_view.projection_version")

		dismissSQL := mock.execCalls[2].SQL
		assert.Contains(t, dismissSQL, "khi.projection_version = "+activeProjectionVersionSQL)
		assert.NotContains(t, dismissSQL, "recall_candidate_view.projection_version")
	})

	t.Run("cutover scenario succeeds when home item exists only at new projection_version", func(t *testing.T) {
		const (
			existingCandidateVersion = 1
			activeVersion            = 2
		)
		homeItemsByVersion := map[int]uuid.UUID{
			activeVersion: tenantID,
		}

		// Simulate Postgres evaluating WHERE EXISTS against knowledge_home_items
		// when recall_candidate_view holds a pre-cutover row at version 1 and
		// knowledge_home_items only has the row at version 2.
		mock := &mockPgx{
			execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				var checkedVersion int
				switch {
				case strings.Contains(sql, "khi.projection_version = recall_candidate_view.projection_version"):
					checkedVersion = existingCandidateVersion
				case strings.Contains(sql, "khi.projection_version = EXCLUDED.projection_version"):
					checkedVersion = args[7].(int)
				case strings.Contains(sql, "khi.projection_version = "+activeProjectionVersionSQL):
					checkedVersion = activeVersion
				}
				boundTenant := args[len(args)-1].(uuid.UUID)
				if ownerTenant, ok := homeItemsByVersion[checkedVersion]; ok && ownerTenant == boundTenant {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
				return pgconn.NewCommandTag("UPDATE 0"), nil
			},
		}
		repo := &Repository{pool: mock}

		snoozePayload, err := json.Marshal(map[string]any{
			"user_id":     userID.String(),
			"tenant_id":   tenantID.String(),
			"item_key":    itemKey,
			"until":       "2026-05-05T03:00:00Z",
			"occurred_at": fixedTestTimeRFC3339,
		})
		require.NoError(t, err)
		require.NoError(t, repo.SnoozeRecallCandidate(ctx, snoozePayload),
			"SnoozeRecallCandidate after projection cutover must not return ErrRecallHomeAssociationDenied")

		dismissPayload, err := json.Marshal(map[string]any{
			"user_id":     userID.String(),
			"tenant_id":   tenantID.String(),
			"item_key":    itemKey,
			"occurred_at": fixedTestTimeRFC3339,
		})
		require.NoError(t, err)
		require.NoError(t, repo.DismissRecallCandidate(ctx, dismissPayload),
			"DismissRecallCandidate after projection cutover must not return ErrRecallHomeAssociationDenied")

		upsertPayload, err := json.Marshal(map[string]any{
			"user_id":            userID.String(),
			"tenant_id":          tenantID.String(),
			"item_key":           itemKey,
			"recall_score":       0.85,
			"updated_at":         fixedTestTimeRFC3339,
			"projection_version": activeVersion,
		})
		require.NoError(t, err)
		require.NoError(t, repo.UpsertRecallCandidate(ctx, upsertPayload),
			"UpsertRecallCandidate on conflict after projection cutover must not return ErrRecallHomeAssociationDenied")
	})
}
