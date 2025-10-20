package alt_db

import (
	"alt/domain"
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

func TestAltDBRepository_FetchReadFeedsListCursor_OrdersByReadAt(t *testing.T) {
	// Initialize logger for tests
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	// Test initial fetch (no cursor)
	t.Run("InitialFetch_OrdersByReadAtDesc", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &AltDBRepository{pool: mock}

		// Create context with user
		userID := uuid.New()
		userCtx := &domain.UserContext{
			UserID:    userID,
			Email:     "test@example.com",
			Role:      domain.UserRoleUser,
			TenantID:  uuid.New(),
			LoginAt:   time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		ctx := domain.SetUserContext(context.Background(), userCtx)

		limit := 20

		// Mock the query - the key thing is that ORDER BY should be rs.read_at DESC
		// Use string UUIDs to match how pgxmock handles them
		feedID1 := uuid.New().String()
		feedID2 := uuid.New().String()
		now := time.Now()
		oldTime := now.Add(-1 * time.Hour)

		// ExpectQuery with a pattern that matches our query structure
		// The important part is verifying rs.read_at is in the ORDER BY clause
		mock.ExpectQuery("SELECT.*FROM feeds.*INNER JOIN read_status.*ORDER BY rs.read_at DESC").
			WithArgs(limit, userID).
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at", "updated_at"}).
				AddRow(feedID1, "Recently Read Feed", "desc1", "https://example.com/feed1", now, oldTime, now).
				AddRow(feedID2, "Older Read Feed", "desc2", "https://example.com/feed2", now, oldTime, now.Add(-2*time.Hour)))

		feeds, err := repo.FetchReadFeedsListCursor(ctx, nil, limit)

		require.NoError(t, err)
		require.Len(t, feeds, 2)
		require.Equal(t, feedID1, feeds[0].ID)
		require.Equal(t, "Recently Read Feed", feeds[0].Title)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// Test cursor-based fetch
	t.Run("CursorFetch_OrdersByReadAtDesc", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		repo := &AltDBRepository{pool: mock}

		// Create context with user
		userID := uuid.New()
		userCtx := &domain.UserContext{
			UserID:    userID,
			Email:     "test@example.com",
			Role:      domain.UserRoleUser,
			TenantID:  uuid.New(),
			LoginAt:   time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		ctx := domain.SetUserContext(context.Background(), userCtx)

		limit := 20
		cursor := time.Now().Add(-1 * time.Hour)

		feedID := uuid.New().String()
		now := time.Now()

		// Expected query should use rs.read_at < $1 and ORDER BY rs.read_at DESC
		// Use AnyArg() for flexibility in argument matching since cursor format can vary
		mock.ExpectQuery("SELECT.*FROM feeds.*INNER JOIN read_status.*ORDER BY rs.read_at DESC").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "link", "pub_date", "created_at", "updated_at"}).
				AddRow(feedID, "Feed Title", "desc", "https://example.com/feed", now, now, now))

		feeds, err := repo.FetchReadFeedsListCursor(ctx, &cursor, limit)

		require.NoError(t, err)
		require.Len(t, feeds, 1)
		require.Equal(t, feedID, feeds[0].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
