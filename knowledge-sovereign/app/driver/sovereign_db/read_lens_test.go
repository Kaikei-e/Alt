package sovereign_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadLensOperations(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	tenantID := uuid.New()
	lensID := uuid.New()
	versionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("ListLenses scans lens and current version", func(t *testing.T) {
		qText := "golang"
		incRecap := true
		incPulse := false
		sortMode := "score"
		mock := &mockPgx{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					scanFuncs: []func(dest ...any) error{
						func(dest ...any) error {
							*dest[0].(*uuid.UUID) = lensID
							*dest[1].(*uuid.UUID) = userID
							*dest[2].(*uuid.UUID) = tenantID
							*dest[3].(*string) = "Go Lens"
							*dest[4].(*string) = "Desc"
							*dest[5].(*time.Time) = now
							*dest[6].(*time.Time) = now
							*dest[7].(**time.Time) = nil
							*dest[8].(**uuid.UUID) = &versionID
							*dest[9].(**uuid.UUID) = &lensID
							*dest[10].(**time.Time) = &now
							*dest[11].(**string) = &qText
							*dest[12].(*[]byte) = []byte(`["go","backend"]`)
							*dest[13].(*[]byte) = []byte(`["src-1"]`)
							*dest[14].(*[]byte) = []byte(`"7d"`)
							*dest[15].(**bool) = &incRecap
							*dest[16].(**bool) = &incPulse
							*dest[17].(**string) = &sortMode
							*dest[18].(**uuid.UUID) = nil
							return nil
						},
					},
				}, nil
			},
		}
		repo := &Repository{pool: mock}

		lenses, err := repo.ListLenses(ctx, userID)
		require.NoError(t, err)
		require.Len(t, lenses, 1)
		assert.Equal(t, lensID, lenses[0].LensID)
		require.NotNil(t, lenses[0].CurrentVersion)
		assert.Equal(t, "golang", lenses[0].CurrentVersion.QueryText)
		assert.Equal(t, []string{"go", "backend"}, lenses[0].CurrentVersion.TagIDs)
		assert.Equal(t, "7d", lenses[0].CurrentVersion.TimeWindow)
	})

	t.Run("GetLens and GetCurrentLensSelection handle ErrNoRows as nil", func(t *testing.T) {
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				return &mockRow{
					scanFunc: func(_ ...interface{}) error {
						return pgx.ErrNoRows
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		lens, err := repo.GetLens(ctx, lensID)
		require.NoError(t, err)
		assert.Nil(t, lens)

		sel, err := repo.GetCurrentLensSelection(ctx, userID)
		require.NoError(t, err)
		assert.Nil(t, sel)
	})

	t.Run("CreateLens, CreateLensVersion, SelectCurrentLens, ClearCurrentLens execute queries", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		require.NoError(t, repo.CreateLens(ctx, KnowledgeLens{
			LensID:   lensID,
			UserID:   userID,
			TenantID: tenantID,
			Name:     "My Lens",
		}))
		require.NoError(t, repo.CreateLensVersion(ctx, KnowledgeLensVersion{
			LensVersionID: versionID,
			LensID:        lensID,
			TagIDs:        []string{"ai"},
			SourceIDs:     []string{},
			TimeWindow:    "24h",
		}))
		require.NoError(t, repo.SelectCurrentLens(ctx, KnowledgeCurrentLens{
			UserID:        userID,
			LensID:        lensID,
			LensVersionID: versionID,
			SelectedAt:    now,
		}))
		require.NoError(t, repo.ClearCurrentLens(ctx, userID))
		assert.Len(t, mock.execCalls, 4)
	})

	t.Run("ResolveLensFilter resolves explicit lensID", func(t *testing.T) {
		callCount := 0
		mock := &mockPgx{
			queryRowFunc: func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
				callCount++
				if callCount == 1 {
					return &mockRow{
						scanFunc: func(dest ...interface{}) error {
							*dest[0].(*uuid.UUID) = lensID
							*dest[1].(*uuid.UUID) = userID
							*dest[2].(*uuid.UUID) = tenantID
							*dest[3].(*string) = "Lens"
							*dest[4].(*string) = "Desc"
							*dest[5].(*time.Time) = now
							*dest[6].(*time.Time) = now
							return nil
						},
					}
				}
				return &mockRow{
					scanFunc: func(dest ...interface{}) error {
						*dest[0].(*uuid.UUID) = versionID
						*dest[1].(*uuid.UUID) = lensID
						*dest[2].(*time.Time) = now
						*dest[3].(*string) = "rust"
						*dest[4].(*[]byte) = []byte(`["systems"]`)
						*dest[5].(*[]byte) = []byte(`[]`)
						*dest[6].(*[]byte) = []byte(`"7d"`)
						*dest[7].(*bool) = true
						*dest[8].(*bool) = false
						*dest[9].(*string) = "score"
						return nil
					},
				}
			},
		}
		repo := &Repository{pool: mock}

		filter, err := repo.ResolveLensFilter(ctx, userID, &lensID)
		require.NoError(t, err)
		require.NotNil(t, filter)
		assert.Equal(t, "rust", filter.QueryText)
		assert.Equal(t, []string{"systems"}, filter.TagNames)
		assert.Equal(t, "7d", filter.TimeWindow)
	})
}
