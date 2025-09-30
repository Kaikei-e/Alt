package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_SaveArticle_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	articleID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Example Title", "<p>content</p>", "https://example.com/article").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(articleID))

	require.NoError(t, repo.SaveArticle(ctx, "https://example.com/article", "Example Title", "<p>content</p>"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_SaveArticle_UsesURLWhenTitleEmpty(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	articleID := uuid.New()
	url := "https://example.com/article"
	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs(url, "<p>content</p>", url).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(articleID))

	require.NoError(t, repo.SaveArticle(ctx, url, "   ", "<p>content</p>"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_SaveArticle_ReturnsErrorOnQueryFailure(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Example Title", "<p>content</p>", "https://example.com/article").
		WillReturnError(errors.New("db failed"))

	err = repo.SaveArticle(ctx, "https://example.com/article", "Example Title", "<p>content</p>")
	require.Error(t, err)
	require.ErrorContains(t, err, "db failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_SaveArticle_ValidationFailures(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	t.Run("nil repository", func(t *testing.T) {
		var repo *AltDBRepository
		err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.Equal(t, "database connection not available", err.Error())
	})

	t.Run("nil pool", func(t *testing.T) {
		repo := &AltDBRepository{}
		err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.Equal(t, "database connection not available", err.Error())
	})

	t.Run("empty url", func(t *testing.T) {
		repo := &AltDBRepository{pool: mock}
		err := repo.SaveArticle(ctx, "   ", "title", "content")
		require.Error(t, err)
		require.Equal(t, "article url cannot be empty", err.Error())
	})

	t.Run("empty content", func(t *testing.T) {
		repo := &AltDBRepository{pool: mock}
		err := repo.SaveArticle(ctx, "https://example.com", "title", "   ")
		require.Error(t, err)
		require.Equal(t, "article content cannot be empty", err.Error())
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
