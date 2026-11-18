package sovereign_db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertReadingSource_SQLAndArgs(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	t0 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	err := repo.UpsertReadingSource(context.Background(), ReadingSource{
		UserID:          userID,
		TenantID:        tenantID,
		ArticleID:       articleID,
		FirstReadAt:     t0,
		ReadRuleVersion: 1,
	})
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)
	sql := mock.execCalls[0].SQL
	assert.Contains(t, sql, "INSERT INTO reading_sources")
	assert.Contains(t, sql, "WHEN EXCLUDED.read_rule_version > reading_sources.read_rule_version THEN EXCLUDED.first_read_at")
	assert.Contains(t, sql, "WHEN EXCLUDED.read_rule_version < reading_sources.read_rule_version THEN reading_sources.first_read_at")
	assert.Contains(t, sql, "ELSE LEAST(reading_sources.first_read_at, EXCLUDED.first_read_at)")
	assert.Contains(t, sql, "GREATEST(reading_sources.read_rule_version, EXCLUDED.read_rule_version)")
	assert.Equal(t, []any{userID, tenantID, articleID, t0, 1}, mock.execCalls[0].Args)
}

func TestUpsertReadingSource_MergeSemantics(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()

	tOld := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	tEarly := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	tLate := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		sequence    []ReadingSource
		wantTime    time.Time
		wantVersion int
	}{
		{
			name: "legacy then v1 takes v1 first_read_at and version",
			sequence: []ReadingSource{
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tOld, ReadRuleVersion: 0},
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tEarly, ReadRuleVersion: 1},
			},
			wantTime:    tEarly,
			wantVersion: 1,
		},
		{
			name: "v1 then legacy (replay order reversed) keeps v1 row unchanged",
			sequence: []ReadingSource{
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tEarly, ReadRuleVersion: 1},
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tOld, ReadRuleVersion: 0},
			},
			wantTime:    tEarly,
			wantVersion: 1,
		},
		{
			name: "v1 twice earlier then later takes earliest first_read_at",
			sequence: []ReadingSource{
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tEarly, ReadRuleVersion: 1},
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tLate, ReadRuleVersion: 1},
			},
			wantTime:    tEarly,
			wantVersion: 1,
		},
		{
			name: "v1 twice later then earlier takes earliest first_read_at",
			sequence: []ReadingSource{
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tLate, ReadRuleVersion: 1},
				{UserID: userID, TenantID: tenantID, ArticleID: articleID, FirstReadAt: tEarly, ReadRuleVersion: 1},
			},
			wantTime:    tEarly,
			wantVersion: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stored *ReadingSource
			mock := &mockPgx{
				execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					require.Len(t, args, 5)
					incoming := ReadingSource{
						UserID:          args[0].(uuid.UUID),
						TenantID:        args[1].(uuid.UUID),
						ArticleID:       args[2].(uuid.UUID),
						FirstReadAt:     args[3].(time.Time),
						ReadRuleVersion: args[4].(int),
					}
					if stored == nil {
						cp := incoming
						stored = &cp
						return pgconn.NewCommandTag("INSERT 0 1"), nil
					}
					normalized := strings.Join(strings.Fields(sql), " ")
					if strings.Contains(normalized, "WHEN EXCLUDED.read_rule_version > reading_sources.read_rule_version THEN EXCLUDED.first_read_at") &&
						strings.Contains(normalized, "WHEN EXCLUDED.read_rule_version < reading_sources.read_rule_version THEN reading_sources.first_read_at") &&
						strings.Contains(normalized, "ELSE LEAST(reading_sources.first_read_at, EXCLUDED.first_read_at)") {
						switch {
						case incoming.ReadRuleVersion > stored.ReadRuleVersion:
							stored.FirstReadAt = incoming.FirstReadAt
							stored.ReadRuleVersion = incoming.ReadRuleVersion
						case incoming.ReadRuleVersion < stored.ReadRuleVersion:
							// Keep stored row unchanged.
						default:
							if incoming.FirstReadAt.Before(stored.FirstReadAt) {
								stored.FirstReadAt = incoming.FirstReadAt
							}
						}
					} else {
						// Independent LEAST/GREATEST fallback models the legacy buggy SQL.
						if incoming.FirstReadAt.Before(stored.FirstReadAt) {
							stored.FirstReadAt = incoming.FirstReadAt
						}
						if incoming.ReadRuleVersion > stored.ReadRuleVersion {
							stored.ReadRuleVersion = incoming.ReadRuleVersion
						}
					}
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				},
			}
			repo := NewRepository(mock)

			for _, src := range tc.sequence {
				require.NoError(t, repo.UpsertReadingSource(context.Background(), src))
			}

			require.NotNil(t, stored)
			assert.Equal(t, tc.wantTime, stored.FirstReadAt)
			assert.Equal(t, tc.wantVersion, stored.ReadRuleVersion)
		})
	}
}

func TestUpsertReadingQueueItem_SQLAndArgs(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	tenantID := uuid.New()
	articleID := uuid.New()
	queuedAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	expiresAt := queuedAt.Add(48 * time.Hour)

	err := repo.UpsertReadingQueueItem(context.Background(), ReadingQueueItem{
		UserID:             userID,
		TenantID:           tenantID,
		ArticleID:          articleID,
		QueuedAt:           queuedAt,
		ExpiresAt:          expiresAt,
		QueuePolicyVersion: 1,
		LastEventSeq:       42,
	})
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)
	assert.Contains(t, mock.execCalls[0].SQL, "INSERT INTO reading_queue_items")
	assert.Contains(t, mock.execCalls[0].SQL, "WHERE reading_queue_items.last_event_seq <= EXCLUDED.last_event_seq")
	assert.Equal(t, []any{userID, tenantID, articleID, queuedAt, expiresAt, 1, int64(42)}, mock.execCalls[0].Args)
}

func TestRemoveReadingQueueItem_SQLAndArgs(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	articleID := uuid.New()
	removedAt := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	err := repo.RemoveReadingQueueItem(context.Background(), userID, articleID, removedAt, "read", 55)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)
	assert.Contains(t, mock.execCalls[0].SQL, "UPDATE reading_queue_items")
	assert.Contains(t, mock.execCalls[0].SQL, "AND removed_at IS NULL")
	assert.Contains(t, mock.execCalls[0].SQL, "AND last_event_seq <= $5")
	assert.Equal(t, []any{userID, articleID, removedAt, "read", int64(55)}, mock.execCalls[0].Args)
}

func TestGetActiveReadingQueue_FiltersRemovedAndExpiresAsOf(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	asOf := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	_, err := repo.GetActiveReadingQueue(context.Background(), userID, asOf)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	assert.Contains(t, mock.queryCalls[0].SQL, "FROM reading_queue_items")
	assert.Contains(t, mock.queryCalls[0].SQL, "removed_at IS NULL")
	assert.Contains(t, mock.queryCalls[0].SQL, "expires_at > $2")
	assert.Contains(t, mock.queryCalls[0].SQL, "ORDER BY queued_at ASC, article_id ASC")
	assert.Equal(t, []any{userID, asOf}, mock.queryCalls[0].Args)
}

func TestCountReadingSourcesSince_FiltersVersionAndSince(t *testing.T) {
	mock := &mockPgx{
		queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
			return &mockRow{scanFunc: func(dest ...interface{}) error {
				*dest[0].(*int) = 4
				return nil
			}}
		},
	}
	repo := NewRepository(mock)
	userID := uuid.New()
	since := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	got, err := repo.CountReadingSourcesSince(context.Background(), userID, since)
	require.NoError(t, err)
	assert.Equal(t, 4, got)
	require.Len(t, mock.queryRowCalls, 1)
	assert.Contains(t, mock.queryRowCalls[0].SQL, "read_rule_version >= 1")
	assert.Contains(t, mock.queryRowCalls[0].SQL, "first_read_at >= $2")
	assert.Equal(t, []any{userID, since}, mock.queryRowCalls[0].Args)
}

func TestListReadingSourcesInWindow_QueriesWindowAndReturnsAllVersions(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)
	userID := uuid.New()
	from := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	_, err := repo.ListReadingSourcesInWindow(context.Background(), userID, from, to)
	require.NoError(t, err)
	require.Len(t, mock.queryCalls, 1)
	sql := mock.queryCalls[0].SQL
	assert.Contains(t, sql, "FROM reading_sources")
	assert.Contains(t, sql, "user_id = $1")
	assert.Contains(t, sql, "first_read_at >= $2")
	assert.Contains(t, sql, "first_read_at <= $3")
	assert.NotContains(t, sql, "read_rule_version >= 1")
	assert.Equal(t, []any{userID, from, to}, mock.queryCalls[0].Args)
}
