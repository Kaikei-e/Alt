package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixedTestTimeRFC3339 = "2026-05-04T03:00:00.000000000Z"

func TestUpsertKnowledgeHomeItem_UsesMergeSafeSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

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
		"freshness_at": "` + fixedTestTimeRFC3339 + `",
		"generated_at": "` + fixedTestTimeRFC3339 + `",
		"updated_at": "` + fixedTestTimeRFC3339 + `",
		"projection_version": 7,
		"summary_state": "pending",
		"url": "https://example.com/x"
	}`)

	require.NoError(t, repo.UpsertKnowledgeHomeItem(context.Background(), json.RawMessage(payload)))
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL

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

func scoreOpUpsertPayload(t *testing.T, scoreOp *string) []byte {
	t.Helper()
	fields := map[string]any{
		"user_id":            "11111111-1111-4111-8111-111111111111",
		"tenant_id":          "22222222-2222-4222-8222-222222222222",
		"item_key":           "article:33333333-3333-4333-8333-333333333333",
		"item_type":          "article",
		"score":              0.1,
		"generated_at":       fixedTestTimeRFC3339,
		"updated_at":         fixedTestTimeRFC3339,
		"projection_version": 1,
	}
	if scoreOp != nil {
		fields["score_op"] = *scoreOp
	}
	raw, err := json.Marshal(fields)
	require.NoError(t, err)
	return raw
}

var scoreCasePattern = regexp.MustCompile(
	`(?s)score = CASE\s*WHEN \$23 = '([a-z]*)' THEN (EXCLUDED\.score)\s*` +
		`WHEN \$23 = '([a-z]*)' THEN (GREATEST\(EXCLUDED\.score, knowledge_home_items\.score\))\s*` +
		`ELSE (knowledge_home_items\.score)\s*END,`)

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

func TestUpsertKnowledgeHomeItem_EmptyScoreOpIsAcceptedAsExplicitNoop(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	emptyOp := ""
	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &emptyOp))
	require.NoError(t, err, "an explicitly empty score_op must be accepted (it is how no-touch folds signal intent)")
	require.Len(t, mock.execCalls, 1)
	assert.Equal(t, "", mock.execCalls[0].Args[22])
}

func TestUpsertKnowledgeHomeItem_RejectsMissingScoreOp(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, nil))
	require.Error(t, err, "a payload with no score_op key at all must fail loudly, not silently discard the score")
	assert.Empty(t, mock.execCalls, "no write should reach the database with an unresolved score_op")
}

func TestUpsertKnowledgeHomeItem_RejectsUnrecognizedScoreOp(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	bogusOp := "increment"
	err := repo.UpsertKnowledgeHomeItem(context.Background(), scoreOpUpsertPayload(t, &bogusOp))
	require.Error(t, err, "an unrecognized score_op must fail loudly, not silently discard the score")
	assert.Empty(t, mock.execCalls)
}

func TestDismissKnowledgeHomeItem_RequiresDismissedAtInPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:test",
		"projection_version": 1
	}`)

	err := repo.DismissKnowledgeHomeItem(context.Background(), payload)
	require.Error(t, err, "missing dismissed_at must error loudly, not fabricate timestamp")
	assert.Empty(t, mock.execCalls, "no UPDATE should be issued when dismissed_at is missing")
}

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

func TestDismissKnowledgeHomeItem_NotFound(t *testing.T) {
	mock := &mockPgx{
		execFunc: func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:missing",
		"projection_version": 1,
		"dismissed_at": "2026-05-04T03:00:00Z"
	}`)

	err := repo.DismissKnowledgeHomeItem(context.Background(), payload)
	require.True(t, errors.Is(err, ErrDismissTargetNotFound))
}

func TestClearSupersedeState_Success(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	payload := json.RawMessage(`{
		"user_id": "11111111-1111-4111-8111-111111111111",
		"item_key": "article:superseded",
		"projection_version": 2
	}`)

	err := repo.ClearSupersedeState(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)
	assert.Contains(t, mock.execCalls[0].SQL, "UPDATE knowledge_home_items")
	assert.Equal(t, uuid.MustParse("11111111-1111-4111-8111-111111111111"), mock.execCalls[0].Args[0])
	assert.Equal(t, "article:superseded", mock.execCalls[0].Args[1])
	assert.Equal(t, 2, mock.execCalls[0].Args[2])
}

func TestParseHomeItemMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m homeItemMutation)
	}{
		{
			name: "valid with scoreOp max",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"tenant_id": "22222222-2222-4222-8222-222222222222",
				"item_key": "article:1",
				"item_type": "article",
				"score": 0.8,
				"score_op": "max",
				"supersede_state": "superseded",
				"previous_ref_json": "{\"prev\":\"old\"}"
			}`,
			check: func(t *testing.T, m homeItemMutation) {
				assert.Equal(t, "max", m.ScoreOp)
				assert.Equal(t, "[]", m.TagsJSON)
				assert.Equal(t, "[]", m.WhyJSON)
				require.NotNil(t, m.SupersedeState)
				assert.Equal(t, "superseded", *m.SupersedeState)
				require.NotNil(t, m.PreviousRefJSON)
				assert.Equal(t, "{\"prev\":\"old\"}", *m.PreviousRefJSON)
			},
		},
		{
			name: "valid with explicit empty scoreOp",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"tenant_id": "22222222-2222-4222-8222-222222222222",
				"item_key": "article:2",
				"score_op": ""
			}`,
			check: func(t *testing.T, m homeItemMutation) {
				assert.Equal(t, "", m.ScoreOp)
				assert.Nil(t, m.SupersedeState)
				assert.Nil(t, m.PreviousRefJSON)
			},
		},
		{
			name:       "missing scoreOp",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111"}`,
			wantErrMsg: "score_op is required",
		},
		{
			name:       "unrecognized scoreOp",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "score_op": "bogus"}`,
			wantErrMsg: "unrecognized score_op \"bogus\"",
		},
		{
			name:       "invalid json",
			payload:    `{invalid`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseHomeItemMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				args := buildUpsertKnowledgeHomeItemArgs(m)
				assert.Len(t, args, 23)
			}
		})
	}
}

func TestParseDismissHomeItemMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
		check      func(t *testing.T, m dismissHomeItemMutation)
	}{
		{
			name: "valid exact version",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:item",
				"projection_version": 3,
				"dismissed_at": "2026-05-04T03:00:00Z"
			}`,
			check: func(t *testing.T, m dismissHomeItemMutation) {
				assert.Equal(t, 3, m.ProjectionVersion)
				assert.Equal(t, "article:item", m.ItemKey)
				assert.Equal(t, time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC), m.DismissedAt)
			},
		},
		{
			name: "valid all versions",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:all",
				"dismissed_at": "2026-05-04T03:00:00Z"
			}`,
			check: func(t *testing.T, m dismissHomeItemMutation) {
				assert.Equal(t, 0, m.ProjectionVersion)
			},
		},
		{
			name:       "missing dismissed_at",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "article:x"}`,
			wantErrMsg: "dismissed_at is required",
		},
		{
			name:       "invalid user_id",
			payload:    `{"user_id": "not-a-uuid", "item_key": "article:x", "dismissed_at": "2026-05-04T03:00:00Z"}`,
			wantErrMsg: "parse user_id",
		},
		{
			name:       "invalid dismissed_at format",
			payload:    `{"user_id": "11111111-1111-4111-8111-111111111111", "item_key": "article:x", "dismissed_at": "bad-date"}`,
			wantErrMsg: "parse dismissed_at",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseDismissHomeItemMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, m)
				}
				query, args := buildDismissHomeItemQueryAndArgs(m)
				assert.NotEmpty(t, query)
				if m.ProjectionVersion == 0 {
					assert.Len(t, args, 3)
				} else {
					assert.Len(t, args, 4)
				}
			}
		})
	}
}

func TestParseClearSupersedeStateMutation_Table(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantErrMsg string
	}{
		{
			name: "valid payload",
			payload: `{
				"user_id": "11111111-1111-4111-8111-111111111111",
				"item_key": "article:clear",
				"projection_version": 1
			}`,
		},
		{
			name:       "invalid user_id",
			payload:    `{"user_id": "bad-uuid", "item_key": "k", "projection_version": 1}`,
			wantErrMsg: "parse user_id",
		},
		{
			name:       "invalid json",
			payload:    `{bad`,
			wantErrMsg: "unmarshal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseClearSupersedeStateMutation(json.RawMessage(tc.payload))
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				args := buildClearSupersedeStateArgs(m)
				assert.Len(t, args, 3)
			}
		})
	}
}
