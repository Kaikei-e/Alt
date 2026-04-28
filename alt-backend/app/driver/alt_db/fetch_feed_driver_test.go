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

		repo := &FeedRepository{pool: mock}

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
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "website_url", "pub_date", "created_at", "updated_at"}).
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

		repo := &FeedRepository{pool: mock}

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
			WillReturnRows(pgxmock.NewRows([]string{"id", "title", "description", "website_url", "pub_date", "created_at", "updated_at"}).
				AddRow(feedID, "Feed Title", "desc", "https://example.com/feed", now, now, now))

		feeds, err := repo.FetchReadFeedsListCursor(ctx, &cursor, limit)

		require.NoError(t, err)
		require.Len(t, feeds, 1)
		require.Equal(t, feedID, feeds[0].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestAltDBRepository_GetAllReadFeedIDs_QueriesWithoutFeedIDArray(t *testing.T) {
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	userID := uuid.New()
	feedID1 := uuid.New()
	feedID2 := uuid.New()

	mock.ExpectQuery("SELECT feed_id FROM read_status").
		WithArgs(userID, 10000).
		WillReturnRows(
			pgxmock.NewRows([]string{"feed_id"}).
				AddRow(feedID1).
				AddRow(feedID2),
		)

	got, err := repo.GetAllReadFeedIDs(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got[feedID1])
	require.True(t, got[feedID2])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildExcludeClauseMultiple(t *testing.T) {
	t.Run("NilSlice_ReturnsEmpty", func(t *testing.T) {
		args := []any{20, uuid.New()}
		clause, resultArgs := buildExcludeClauseMultiple(args, nil)
		require.Equal(t, "", clause)
		require.Len(t, resultArgs, 2)
	})

	t.Run("EmptySlice_ReturnsEmpty", func(t *testing.T) {
		args := []any{20, uuid.New()}
		clause, resultArgs := buildExcludeClauseMultiple(args, []uuid.UUID{})
		require.Equal(t, "", clause)
		require.Len(t, resultArgs, 2)
	})

	t.Run("SingleElement_GeneratesClause", func(t *testing.T) {
		id := uuid.New()
		args := []any{20, uuid.New()}
		clause, resultArgs := buildExcludeClauseMultiple(args, []uuid.UUID{id})
		require.Equal(t, "AND f.feed_link_id != ALL($3::uuid[])", clause)
		require.Len(t, resultArgs, 3)
		require.Equal(t, []string{id.String()}, resultArgs[2])
	})

	t.Run("MultipleElements_GeneratesClause", func(t *testing.T) {
		ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
		args := []any{20, uuid.New()}
		clause, resultArgs := buildExcludeClauseMultiple(args, ids)
		require.Equal(t, "AND f.feed_link_id != ALL($3::uuid[])", clause)
		require.Len(t, resultArgs, 3)
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = id.String()
		}
		require.Equal(t, strs, resultArgs[2])
	})

	t.Run("PreservesExistingArgs", func(t *testing.T) {
		existingTime := time.Now()
		existingUser := uuid.New()
		args := []any{existingTime, 20, existingUser}
		ids := []uuid.UUID{uuid.New()}
		clause, resultArgs := buildExcludeClauseMultiple(args, ids)
		require.Equal(t, "AND f.feed_link_id != ALL($4::uuid[])", clause)
		require.Len(t, resultArgs, 4)
		require.Equal(t, existingTime, resultArgs[0])
		require.Equal(t, 20, resultArgs[1])
		require.Equal(t, existingUser, resultArgs[2])
	})
}

func TestAltDBRepository_GetReadFeedIDs_UsesUUIDArrayCast(t *testing.T) {
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Logger = testLogger

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &FeedRepository{pool: mock}
	userID := uuid.New()
	feedID := uuid.New()

	mock.ExpectQuery("SELECT feed_id FROM read_status").
		WithArgs(userID, []string{feedID.String()}).
		WillReturnRows(
			pgxmock.NewRows([]string{"feed_id"}).
				AddRow(feedID),
		)

	got, err := repo.GetReadFeedIDs(context.Background(), userID, []uuid.UUID{feedID})
	require.NoError(t, err)
	require.True(t, got[feedID])
	require.NoError(t, mock.ExpectationsWereMet())
}
