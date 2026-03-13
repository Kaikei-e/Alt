package alt_db

import (
	"alt/utils/logger"
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestFetchArticlesByIDs_PassesStringSliceForPgBouncerCompat(t *testing.T) {
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	id1 := uuid.New()
	id2 := uuid.New()

	now := time.Now()

	// Expect the query to receive []string (not []uuid.UUID)
	// This is required for PgBouncer simple protocol compatibility
	mock.ExpectQuery("SELECT").
		WithArgs([]string{id1.String(), id2.String()}).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "feed_id", "title", "content", "url", "created_at", "tags"}).
				AddRow(id1, uuid.New(), "Article 1", "Content 1", "http://example.com/1", now, []string{"tag1"}).
				AddRow(id2, uuid.New(), "Article 2", "Content 2", "http://example.com/2", now, []string{"tag2"}),
		)

	articles, err := repo.FetchArticlesByIDs(context.Background(), []uuid.UUID{id1, id2})
	require.NoError(t, err)
	require.Len(t, articles, 2)
	require.Equal(t, id1, articles[0].ID)
	require.Equal(t, id2, articles[1].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByIDs_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	articles, fetchErr := repo.FetchArticlesByIDs(context.Background(), []uuid.UUID{})
	require.NoError(t, fetchErr)
	require.Empty(t, articles)
}

func TestFetchArticlesByIDs_NilPool(t *testing.T) {
	repo := &AltDBRepository{pool: nil}

	_, err := repo.FetchArticlesByIDs(context.Background(), []uuid.UUID{uuid.New()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "database connection not available")
}
