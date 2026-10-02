package sovereign_db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDismissKnowledgeHomeItem_TenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()
	dismissedAt := time.Now().UTC().Format(time.RFC3339Nano)

	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:dismiss-1",
		"projection_version": 2,
		"dismissed_at":       dismissedAt,
	})
	require.NoError(t, err)

	err = repo.DismissKnowledgeHomeItem(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "AND tenant_id = $5", "DismissKnowledgeHomeItem must enforce tenant_id in SQL WHERE clause")
	require.Len(t, args, 5)
	assert.Equal(t, tenantID, args[4])
}

func TestClearSupersedeState_TenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()

	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:clear-1",
		"projection_version": 1,
	})
	require.NoError(t, err)

	err = repo.ClearSupersedeState(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "AND tenant_id = $4", "ClearSupersedeState must enforce tenant_id in SQL WHERE clause")
	require.Len(t, args, 4)
	assert.Equal(t, tenantID, args[3])
}

func TestPatchKnowledgeHomeItemURL_TenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()

	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:patch-url-1",
		"projection_version": 3,
		"url":                "https://example.com/item1",
	})
	require.NoError(t, err)

	err = repo.PatchKnowledgeHomeItemURL(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "AND tenant_id = $5", "PatchKnowledgeHomeItemURL must enforce tenant_id in SQL WHERE clause")
	require.Len(t, args, 5)
	assert.Equal(t, tenantID, args[4])
}

func TestSnoozeRecallCandidate_HomeAssociationTenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC()
	occurredAt := now.Format(time.RFC3339Nano)
	until := now.Add(24 * time.Hour).Format(time.RFC3339Nano)

	payload, err := json.Marshal(map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    "article:snooze-guard-1",
		"occurred_at": occurredAt,
		"until":       until,
	})
	require.NoError(t, err)

	err = repo.SnoozeRecallCandidate(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "knowledge_home_items", "SnoozeRecallCandidate must guard against knowledge_home_items")
	assert.Contains(t, sql, "khi.tenant_id = $5", "SnoozeRecallCandidate must check home item tenant_id")
	assert.Contains(t, sql, "khi.projection_version = recall_candidate_view.projection_version", "SnoozeRecallCandidate must check projection_version association")
	require.Len(t, args, 5)
	assert.Equal(t, tenantID, args[4])
}

func TestDismissRecallCandidate_HomeAssociationTenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC()
	occurredAt := now.Format(time.RFC3339Nano)

	payload, err := json.Marshal(map[string]any{
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"item_key":    "article:dismiss-guard-1",
		"occurred_at": occurredAt,
	})
	require.NoError(t, err)

	err = repo.DismissRecallCandidate(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "knowledge_home_items", "DismissRecallCandidate must guard against knowledge_home_items")
	assert.Contains(t, sql, "khi.tenant_id = $4", "DismissRecallCandidate must check home item tenant_id")
	assert.Contains(t, sql, "khi.projection_version = recall_candidate_view.projection_version", "DismissRecallCandidate must check projection_version association")
	require.Len(t, args, 4)
	assert.Equal(t, tenantID, args[3])
}

func TestRecallCandidate_HomeAssociationIsolation_Scenarios(t *testing.T) {
	tests := []struct {
		name                 string
		homeAssociationMatch bool
		wantRowsAffected     int64
	}{
		{
			name:                 "home association present and matching (own row positive)",
			homeAssociationMatch: true,
			wantRowsAffected:     1,
		},
		{
			name:                 "home association missing / foreign tenant (zero changes)",
			homeAssociationMatch: false,
			wantRowsAffected:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockPgx{}
			mock.execFunc = func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
				if tc.homeAssociationMatch {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			repo := &Repository{pool: mock}

			userID := uuid.New()
			tenantID := uuid.New()
			now := time.Now().UTC()

			snoozePayload, err := json.Marshal(map[string]any{
				"user_id":     userID.String(),
				"tenant_id":   tenantID.String(),
				"item_key":    "article:isolation-test",
				"occurred_at": now.Format(time.RFC3339Nano),
				"until":       now.Add(time.Hour).Format(time.RFC3339Nano),
			})
			require.NoError(t, err)

			err = repo.SnoozeRecallCandidate(context.Background(), snoozePayload)
			if tc.wantRowsAffected == 0 {
				require.ErrorIs(t, err, ErrRecallHomeAssociationDenied)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, mock.execCalls, 1)

			dismissPayload, err := json.Marshal(map[string]any{
				"user_id":     userID.String(),
				"tenant_id":   tenantID.String(),
				"item_key":    "article:isolation-test",
				"occurred_at": now.Format(time.RFC3339Nano),
			})
			require.NoError(t, err)

			err = repo.DismissRecallCandidate(context.Background(), dismissPayload)
			if tc.wantRowsAffected == 0 {
				require.ErrorIs(t, err, ErrRecallHomeAssociationDenied)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, mock.execCalls, 2)
		})
	}
}

func TestUpsertKnowledgeHomeItem_TenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()

	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:home-upsert-1",
		"item_type":          "article",
		"score_op":           "set",
		"score":              0.5,
		"generated_at":       fixedTestTimeRFC3339,
		"updated_at":         fixedTestTimeRFC3339,
		"projection_version": 1,
	})
	require.NoError(t, err)

	err = repo.UpsertKnowledgeHomeItem(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	assert.Contains(t, sql, "WHERE knowledge_home_items.tenant_id = EXCLUDED.tenant_id",
		"UpsertKnowledgeHomeItem ON CONFLICT DO UPDATE MUST enforce stored tenant matches incoming tenant")

	// Test foreign existing row rejected
	mockForeign := &mockPgx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	repoForeign := &Repository{pool: mockForeign}
	err = repoForeign.UpsertKnowledgeHomeItem(context.Background(), payload)
	require.Error(t, err, "foreign existing row with 0 rows affected MUST return tenant mismatch error")
	assert.ErrorIs(t, err, ErrHomeItemTenantMismatch)
}

func TestUpsertRecallCandidate_HomeAssociationTenantGuardSQL(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()

	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"tenant_id":          tenantID.String(),
		"item_key":           "article:cand-guard-1",
		"recall_score":       0.8,
		"projection_version": 2,
		"updated_at":         fixedTestTimeRFC3339,
	})
	require.NoError(t, err)

	err = repo.UpsertRecallCandidate(context.Background(), payload)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	args := mock.execCalls[0].Args

	assert.Contains(t, sql, "knowledge_home_items", "UpsertRecallCandidate must check knowledge_home_items association")
	assert.Contains(t, sql, "khi.tenant_id = $9", "UpsertRecallCandidate must check home item tenant_id")
	assert.Contains(t, sql, "khi.projection_version = $8", "UpsertRecallCandidate must check incoming candidate projection_version association")
	assert.Contains(t, sql, "khi.projection_version = recall_candidate_view.projection_version", "UpsertRecallCandidate must check existing candidate projection_version association on conflict update")
	require.Len(t, args, 9)
	assert.Equal(t, tenantID, args[8])

	// Test foreign/missing home row rejected on initial insert (INSERT 0 0)
	mockDeniedInsert := &mockPgx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 0"), nil
		},
	}
	repoDeniedInsert := &Repository{pool: mockDeniedInsert}
	err = repoDeniedInsert.UpsertRecallCandidate(context.Background(), payload)
	require.Error(t, err, "foreign or missing home row resulting in 0 rows affected on insert MUST be denied")
	assert.ErrorIs(t, err, ErrRecallHomeAssociationDenied)

	// Test Own Incoming / Foreign Existing: on conflict DO UPDATE WHERE EXISTS fails (UPDATE 0) -> denied
	mockDeniedUpdate := &mockPgx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	repoDeniedUpdate := &Repository{pool: mockDeniedUpdate}
	err = repoDeniedUpdate.UpsertRecallCandidate(context.Background(), payload)
	require.Error(t, err, "foreign existing home row resulting in 0 rows affected on update MUST be denied")
	assert.ErrorIs(t, err, ErrRecallHomeAssociationDenied)

	// Test Own Existing positive: on conflict DO UPDATE WHERE EXISTS succeeds (UPDATE 1)
	mockOwnExisting := &mockPgx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	repoOwnExisting := &Repository{pool: mockOwnExisting}
	err = repoOwnExisting.UpsertRecallCandidate(context.Background(), payload)
	require.NoError(t, err, "own existing candidate with matching home association MUST succeed")
}
