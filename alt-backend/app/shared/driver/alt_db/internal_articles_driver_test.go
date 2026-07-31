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
