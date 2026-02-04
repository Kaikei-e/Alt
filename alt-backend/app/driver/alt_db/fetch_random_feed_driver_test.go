package alt_db

import (
	"alt/utils/logger"
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_FetchRandomFeed(t *testing.T) {
	// Initialize logger for tests
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	t.Run("successfully fetches a random feed from feeds table", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &AltDBRepository{pool: mock}
		ctx := context.Background()

		feedID := uuid.New()
		expectedTitle := "Test Feed Title"
		expectedDescription := "Test feed description"
		expectedLink := "https://example.com"

		mock.ExpectQuery("SELECT id, title, description, link FROM feeds").
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "link"}).
				AddRow(feedID, expectedTitle, expectedDescription, expectedLink))

		feed, err := repo.FetchRandomFeed(ctx)

		require.NoError(t, err)
		require.NotNil(t, feed)
		require.Equal(t, feedID, feed.ID)
		require.Equal(t, expectedTitle, feed.Title)
		require.Equal(t, expectedDescription, feed.Description)
		require.Equal(t, expectedLink, feed.Link)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns nil when no feeds exist", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &AltDBRepository{pool: mock}
		ctx := context.Background()

		mock.ExpectQuery("SELECT id, title, description, link FROM feeds").
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "link"}))

		feed, err := repo.FetchRandomFeed(ctx)

		require.NoError(t, err)
		require.Nil(t, feed)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles null description gracefully", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &AltDBRepository{pool: mock}
		ctx := context.Background()

		feedID := uuid.New()
		expectedTitle := "Test Feed"
		expectedLink := "https://example.com"

		mock.ExpectQuery("SELECT id, title, description, link FROM feeds").
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "link"}).
				AddRow(feedID, expectedTitle, nil, expectedLink))

		feed, err := repo.FetchRandomFeed(ctx)

		require.NoError(t, err)
		require.NotNil(t, feed)
		require.Equal(t, feedID, feed.ID)
		require.Equal(t, expectedTitle, feed.Title)
		require.Equal(t, "", feed.Description) // nil becomes empty string
		require.Equal(t, expectedLink, feed.Link)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
