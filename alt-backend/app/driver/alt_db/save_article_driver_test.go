package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_SaveArticle_Success(t *testing.T) {
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

	// Mock GetFeedIDByArticleURL call - feed not found (will use NULL feed_id)
	mock.ExpectQuery(`SELECT id FROM feeds WHERE link = \$1`).
		WithArgs("https://example.com/article").
		WillReturnError(errors.New("no rows"))

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Example Title", "<p>content</p>", "https://example.com/article", userID, nil).
		WillReturnError(errors.New("db failed"))

	mock.ExpectRollback()

	_, err = repo.SaveArticle(ctx, "https://example.com/article", "Example Title", "<p>content</p>")
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
		_, err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.Equal(t, "database connection not available", err.Error())
	})

	t.Run("nil pool", func(t *testing.T) {
		repo := &AltDBRepository{}
		_, err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.Equal(t, "database connection not available", err.Error())
	})

	// Create context with user for tests that need it
	userID := uuid.New()
	userCtx := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctxWithUser := domain.SetUserContext(context.Background(), userCtx)

	t.Run("empty url", func(t *testing.T) {
		repo := &AltDBRepository{pool: mock}
		_, err := repo.SaveArticle(ctxWithUser, "   ", "title", "content")
		require.Error(t, err)
		require.Equal(t, "article url cannot be empty", err.Error())
	})

	t.Run("empty content", func(t *testing.T) {
		repo := &AltDBRepository{pool: mock}
		_, err := repo.SaveArticle(ctxWithUser, "https://example.com", "title", "   ")
		require.Error(t, err)
		require.Equal(t, "article content cannot be empty", err.Error())
	})

	t.Run("missing user context", func(t *testing.T) {
		repo := &AltDBRepository{pool: mock}
		_, err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.ErrorContains(t, err, "user context required")
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
