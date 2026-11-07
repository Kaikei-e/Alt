package alt_db

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetArticleWithTagsByID_PublishedAt covers the source publication
// timestamp exposed to search-indexer. articles.published_at is nullable, so
// the driver must distinguish "no source timestamp" from a zero instant.
func TestGetArticleWithTagsByID_PublishedAt(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 3, 20, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		column  *time.Time
		wantNil bool
	}{
		{name: "published_at present", column: &publishedAt},
		{name: "published_at is NULL", column: nil, wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			repo := &InternalRepository{pool: mock}

			mock.ExpectQuery("SELECT").
				WithArgs("a1").
				WillReturnRows(pgxmock.NewRows([]string{
					"id", "title", "content", "created_at", "published_at", "user_id", "language", "tag_names",
				}).AddRow("a1", "Test", "Body", createdAt, tt.column, "u1", "en", []string{"go"}))

			article, err := repo.GetArticleWithTagsByID(context.Background(), "a1")
			require.NoError(t, err)
			require.NotNil(t, article)

			assert.Equal(t, createdAt, article.CreatedAt)
			if tt.wantNil {
				assert.Nil(t, article.PublishedAt)
			} else {
				require.NotNil(t, article.PublishedAt)
				assert.Equal(t, publishedAt, *article.PublishedAt)
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestListDeletedArticles_UsesExclusiveCursorAndWithTies(t *testing.T) {
	cursor := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	nextTie := cursor.Add(5 * time.Minute)

	t.Run("initial page uses ORDER BY deleted_at ASC and FETCH FIRST WITH TIES", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &InternalRepository{pool: mock}

		mock.ExpectQuery(`^SELECT id, deleted_at FROM articles WHERE deleted_at IS NOT NULL ORDER BY deleted_at ASC FETCH FIRST \$1 ROWS WITH TIES$`).
			WithArgs(2).
			WillReturnRows(pgxmock.NewRows([]string{"id", "deleted_at"}).
				AddRow("11111111-1111-1111-1111-111111111111", cursor).
				AddRow("22222222-2222-2222-2222-222222222222", nextTie).
				AddRow("33333333-3333-3333-3333-333333333333", nextTie))

		got, finalDeletedAt, err := repo.ListDeletedArticles(context.Background(), nil, 2)
		require.NoError(t, err)
		require.Len(t, got, 3)
		require.NotNil(t, finalDeletedAt)
		assert.Equal(t, nextTie, *finalDeletedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("incremental page uses exclusive deleted_at comparison without empty-string uuid literal and FETCH FIRST WITH TIES", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &InternalRepository{pool: mock}

		mock.ExpectQuery(`^SELECT id, deleted_at FROM articles WHERE deleted_at IS NOT NULL AND deleted_at > \$1 ORDER BY deleted_at ASC FETCH FIRST \$2 ROWS WITH TIES$`).
			WithArgs(cursor, 200).
			WillReturnRows(pgxmock.NewRows([]string{"id", "deleted_at"}).
				AddRow("44444444-4444-4444-4444-444444444444", nextTie))

		got, finalDeletedAt, err := repo.ListDeletedArticles(context.Background(), &cursor, 200)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "44444444-4444-4444-4444-444444444444", got[0].ID)
		require.NotNil(t, finalDeletedAt)
		assert.Equal(t, nextTie, *finalDeletedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
