package sovereign_db

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAreArticlesVisibleInLens(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	userID := uuid.New()
	art1 := uuid.New()
	art2 := uuid.New()

	t.Run("returns empty map without querying when articleIDs is empty", func(t *testing.T) {
		mock := &mockPgx{}
		repo := &Repository{pool: mock}

		res, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, res)
		assert.Empty(t, mock.queryCalls)
	})

	t.Run("applies query_text, tag_names, and time_window filters and maps visible IDs", func(t *testing.T) {
		mock := &mockPgx{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return &mockRows{
					scanFuncs: []func(dest ...any) error{
						func(dest ...any) error {
							*dest[0].(*uuid.UUID) = art1
							return nil
						},
					},
				}, nil
			},
		}
		repo := &Repository{pool: mock}

		filter := &LensFilter{
			QueryText:  "rust",
			TagNames:   []string{"systems"},
			TimeWindow: "7d",
		}
		res, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, []uuid.UUID{art1, art2}, filter)
		require.NoError(t, err)
		assert.True(t, res[art1])
		assert.False(t, res[art2])
		require.Len(t, mock.queryCalls, 1)
		assert.Contains(t, mock.queryCalls[0].SQL, "khi.title ILIKE $4")
		assert.Contains(t, mock.queryCalls[0].SQL, "WHERE tag_name = ANY($5)")
		assert.Contains(t, mock.queryCalls[0].SQL, "AND khi.published_at >= $6")
	})

	t.Run("rejects invalid time_window and propagates query error", func(t *testing.T) {
		mock := &mockPgx{
			queryFunc: func(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
				return nil, errors.New("db query failed")
			},
		}
		repo := &Repository{pool: mock}

		_, err := repo.AreArticlesVisibleInLens(ctx, tenantID, userID, []uuid.UUID{art1}, &LensFilter{TimeWindow: "invalid"})
		require.Error(t, err)

		_, err = repo.AreArticlesVisibleInLens(ctx, tenantID, userID, []uuid.UUID{art1}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AreArticlesVisibleInLens")
	})
}
