package sovereign_db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertReadingThemeVersion_SQLTxAndArgs(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	tenantID := uuid.New()
	themeID := uuid.New()
	artA := uuid.New()
	artB := uuid.New()
	t0 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	err := repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID:        userID,
		TenantID:      tenantID,
		ThemeID:       themeID,
		Title:         "Distributed Consensus",
		LatestVersion: 1,
		ArticleCount:  2,
		CreatedAt:     t0,
		UpdatedAt:     t0,
	}, []uuid.UUID{artA, artB})
	require.NoError(t, err)

	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.committed)
	require.Len(t, mock.execCalls, 3)

	upsertSQL := mock.execCalls[0].SQL
	assert.Contains(t, upsertSQL, "INSERT INTO reading_themes")
	assert.Contains(t, upsertSQL, "ON CONFLICT (user_id, theme_id) DO UPDATE")
	assert.Contains(t, upsertSQL, "WHERE reading_themes.latest_version <= EXCLUDED.latest_version")
	assert.NotContains(t, upsertSQL, "last_opened_at")
	assert.NotContains(t, upsertSQL, "last_opened_version")
	assert.Equal(t, []any{userID, tenantID, themeID, "Distributed Consensus", 1, 2, t0, t0}, mock.execCalls[0].Args)

	deleteSQL := mock.execCalls[1].SQL
	assert.Contains(t, deleteSQL, "DELETE FROM reading_theme_members")
	assert.Contains(t, deleteSQL, "NOT (article_id = ANY($3::uuid[]))")
	assert.Equal(t, []any{userID, themeID, []uuid.UUID{artA, artB}}, mock.execCalls[1].Args)

	insertMembersSQL := mock.execCalls[2].SQL
	assert.Contains(t, insertMembersSQL, "INSERT INTO reading_theme_members")
	assert.Contains(t, insertMembersSQL, "ON CONFLICT (user_id, theme_id, article_id) DO NOTHING")
	assert.Equal(t, []any{userID, tenantID, themeID, 1, []uuid.UUID{artA, artB}}, mock.execCalls[2].Args)
}

func TestUpsertReadingThemeVersion_EmptyMembersDeletesAllAndSkipsInsert(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	tenantID := uuid.New()
	themeID := uuid.New()
	t0 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	err := repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID:        userID,
		TenantID:      tenantID,
		ThemeID:       themeID,
		Title:         "Empty Theme",
		LatestVersion: 2,
		ArticleCount:  0,
		CreatedAt:     t0,
		UpdatedAt:     t0,
	}, nil)
	require.NoError(t, err)

	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.committed)
	require.Len(t, mock.execCalls, 2)
	assert.Contains(t, mock.execCalls[1].SQL, "DELETE FROM reading_theme_members")
	assert.NotContains(t, mock.execCalls[1].SQL, "ANY(")
	assert.Equal(t, []any{userID, themeID}, mock.execCalls[1].Args)
}

func TestUpsertReadingThemeVersion_StaleVersionDoesNotTouchMembers(t *testing.T) {
	mock := &mockPgx{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO reading_themes") {
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	repo := NewRepository(mock)

	err := repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID:        uuid.New(),
		TenantID:      uuid.New(),
		ThemeID:       uuid.New(),
		Title:         "Stale",
		LatestVersion: 1,
		ArticleCount:  1,
		CreatedAt:     time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
	}, []uuid.UUID{uuid.New()})
	require.NoError(t, err)

	require.Len(t, mock.execCalls, 1)
	assert.True(t, mock.lastTx.committed)
}

func TestUpsertReadingThemeVersion_RollsBackOnMemberError(t *testing.T) {
	memberErr := errors.New("write failed")
	mock := &mockPgx{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "DELETE FROM reading_theme_members") {
				return pgconn.CommandTag{}, memberErr
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	repo := NewRepository(mock)

	err := repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID:        uuid.New(),
		TenantID:      uuid.New(),
		ThemeID:       uuid.New(),
		Title:         "Theme",
		LatestVersion: 1,
		ArticleCount:  1,
		CreatedAt:     time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC),
	}, []uuid.UUID{uuid.New()})
	require.ErrorIs(t, err, memberErr)
	require.NotNil(t, mock.lastTx)
	assert.False(t, mock.lastTx.committed)
	assert.True(t, mock.lastTx.rolledBack)
}

func TestUpsertReadingThemeVersion_MergeAndMemberSemantics(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	themeID := uuid.New()
	artA := uuid.New()
	artB := uuid.New()
	artC := uuid.New()

	t1 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	tOpen := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	var stored *ReadingTheme
	members := map[uuid.UUID]int{}

	mock := &mockPgx{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "INSERT INTO reading_themes"):
				incoming := ReadingTheme{
					UserID:        args[0].(uuid.UUID),
					TenantID:      args[1].(uuid.UUID),
					ThemeID:       args[2].(uuid.UUID),
					Title:         args[3].(string),
					LatestVersion: args[4].(int),
					ArticleCount:  args[5].(int),
					CreatedAt:     args[6].(time.Time),
					UpdatedAt:     args[7].(time.Time),
				}
				if stored == nil {
					cp := incoming
					stored = &cp
					return pgconn.NewCommandTag("INSERT 0 1"), nil
				}
				if incoming.LatestVersion < stored.LatestVersion {
					return pgconn.NewCommandTag("INSERT 0 0"), nil
				}
				stored.Title = incoming.Title
				stored.LatestVersion = incoming.LatestVersion
				stored.ArticleCount = incoming.ArticleCount
				stored.UpdatedAt = incoming.UpdatedAt
				return pgconn.NewCommandTag("INSERT 0 1"), nil

			case strings.Contains(sql, "DELETE FROM reading_theme_members"):
				if len(args) == 2 {
					clear(members)
					return pgconn.NewCommandTag("DELETE"), nil
				}
				keep := args[2].([]uuid.UUID)
				for id := range members {
					if !slices.Contains(keep, id) {
						delete(members, id)
					}
				}
				return pgconn.NewCommandTag("DELETE"), nil

			case strings.Contains(sql, "INSERT INTO reading_theme_members"):
				version := args[3].(int)
				for _, id := range args[4].([]uuid.UUID) {
					if _, exists := members[id]; !exists {
						members[id] = version
					}
				}
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	repo := NewRepository(mock)

	require.NoError(t, repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID: userID, TenantID: tenantID, ThemeID: themeID,
		Title: "V1 Title", LatestVersion: 1, ArticleCount: 2, CreatedAt: t1, UpdatedAt: t1,
	}, []uuid.UUID{artA, artB}))

	stored.LastOpenedAt = &tOpen
	stored.LastOpenedVersion = 1

	require.NoError(t, repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID: userID, TenantID: tenantID, ThemeID: themeID,
		Title: "V2 Title", LatestVersion: 2, ArticleCount: 2, CreatedAt: t2, UpdatedAt: t2,
	}, []uuid.UUID{artB, artC}))

	assert.Equal(t, "V2 Title", stored.Title)
	assert.Equal(t, 2, stored.LatestVersion)
	assert.Equal(t, t1, stored.CreatedAt)
	assert.Equal(t, t2, stored.UpdatedAt)
	require.NotNil(t, stored.LastOpenedAt)
	assert.Equal(t, tOpen, *stored.LastOpenedAt)
	assert.Equal(t, 1, stored.LastOpenedVersion)
	assert.Equal(t, map[uuid.UUID]int{artB: 1, artC: 2}, members)

	// Late arrival of version 1 must not mutate theme or members.
	require.NoError(t, repo.UpsertReadingThemeVersion(context.Background(), ReadingTheme{
		UserID: userID, TenantID: tenantID, ThemeID: themeID,
		Title: "V1 Late", LatestVersion: 1, ArticleCount: 2, CreatedAt: t1, UpdatedAt: t1,
	}, []uuid.UUID{artA, artB}))

	assert.Equal(t, "V2 Title", stored.Title)
	assert.Equal(t, 2, stored.LatestVersion)
	assert.Equal(t, map[uuid.UUID]int{artB: 1, artC: 2}, members)
}

func TestMarkReadingThemeOpened_SQLAndArgs(t *testing.T) {
	mock := &mockPgx{}
	repo := NewRepository(mock)

	userID := uuid.New()
	themeID := uuid.New()
	openedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	err := repo.MarkReadingThemeOpened(context.Background(), userID, themeID, openedAt, 3)
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	sql := mock.execCalls[0].SQL
	assert.Contains(t, sql, "UPDATE reading_themes")
	assert.Contains(t, sql, "GREATEST")
	assert.Contains(t, sql, "last_opened_at")
	assert.Contains(t, sql, "last_opened_version")
	assert.Equal(t, []any{userID, themeID, openedAt, 3}, mock.execCalls[0].Args)
}

func TestListReadingThemes_OrdersClampsAndScansNullableLastOpenedAt(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	themeA := uuid.New()
	themeB := uuid.New()
	t1 := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	openedAt := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

	t.Run("default and max limit clamping", func(t *testing.T) {
		mock := &mockPgx{}
		repo := NewRepository(mock)

		_, err := repo.ListReadingThemes(context.Background(), userID, 0)
		require.NoError(t, err)
		require.Len(t, mock.queryCalls, 1)
		assert.Equal(t, []any{userID, 100}, mock.queryCalls[0].Args)
		assert.Contains(t, mock.queryCalls[0].SQL, "FROM reading_themes")
		assert.Contains(t, mock.queryCalls[0].SQL, "ORDER BY updated_at DESC, theme_id ASC")

		_, err = repo.ListReadingThemes(context.Background(), userID, 999)
		require.NoError(t, err)
		require.Len(t, mock.queryCalls, 2)
		assert.Equal(t, []any{userID, 500}, mock.queryCalls[1].Args)
	})

	t.Run("scans rows with nullable last_opened_at", func(t *testing.T) {
		mock := &mockPgx{
			queryFunc: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
				return &mockRows{
					scanFuncs: []func(dest ...any) error{
						func(dest ...any) error {
							*dest[0].(*uuid.UUID) = userID
							*dest[1].(*uuid.UUID) = tenantID
							*dest[2].(*uuid.UUID) = themeA
							*dest[3].(*string) = "Theme A"
							*dest[4].(*int) = 2
							*dest[5].(*int) = 4
							*dest[6].(*time.Time) = t1
							*dest[7].(*time.Time) = t2
							*dest[8].(**time.Time) = &openedAt
							*dest[9].(*int) = 2
							return nil
						},
						func(dest ...any) error {
							*dest[0].(*uuid.UUID) = userID
							*dest[1].(*uuid.UUID) = tenantID
							*dest[2].(*uuid.UUID) = themeB
							*dest[3].(*string) = "Theme B"
							*dest[4].(*int) = 1
							*dest[5].(*int) = 2
							*dest[6].(*time.Time) = t1
							*dest[7].(*time.Time) = t1
							*dest[8].(**time.Time) = nil
							*dest[9].(*int) = 0
							return nil
						},
					},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.ListReadingThemes(context.Background(), userID, 25)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, []any{userID, 25}, mock.queryCalls[0].Args)

		assert.Equal(t, themeA, got[0].ThemeID)
		assert.Equal(t, "Theme A", got[0].Title)
		assert.Equal(t, 2, got[0].LatestVersion)
		assert.Equal(t, 4, got[0].ArticleCount)
		require.NotNil(t, got[0].LastOpenedAt)
		assert.Equal(t, openedAt, *got[0].LastOpenedAt)
		assert.Equal(t, 2, got[0].LastOpenedVersion)

		assert.Equal(t, themeB, got[1].ThemeID)
		assert.Nil(t, got[1].LastOpenedAt)
		assert.Equal(t, 0, got[1].LastOpenedVersion)
	})
}

func TestRebuildTargetReading_IncludesReadingThemesTables(t *testing.T) {
	target, err := LookupRebuildTarget("reading")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"reading_sources",
		"reading_queue_items",
		"reading_themes",
		"reading_theme_members",
		"reading_annotations",
		"reading_theme_curations",
	}, target.Tables())
}
