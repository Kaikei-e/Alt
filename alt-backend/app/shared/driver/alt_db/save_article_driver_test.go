package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_SaveArticle_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

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

	// Mock GetFeedIDByArticleURL call - feed genuinely not found (will use NULL feed_id)
	mock.ExpectQuery(`SELECT id FROM feeds WHERE website_url = \$1`).
		WithArgs("https://example.com/article").
		WillReturnError(pgx.ErrNoRows)

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

func TestAltDBRepository_SaveArticle_UpsertAndOutbox(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	userID := uuid.New()
	tenantID := uuid.New()
	userCtx := &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tenantID,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	mock.ExpectQuery(`SELECT id FROM feeds WHERE website_url = \$1`).
		WithArgs("https://example.com/article").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Example Title", strings.Repeat("x", 120), "https://example.com/article", userID, nil).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow(uuid.New(), true))
	mock.ExpectExec(`INSERT INTO outbox_events`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	_, err = repo.SaveArticle(ctx, "https://example.com/article", "Example Title", strings.Repeat("x", 120))
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Finding [14]: getFeedIDByArticleURL previously collapsed every error
// (pgx.ErrNoRows AND real DB failures like connection timeouts) into the
// same generic "error getting feed ID by article URL" string, and
// save_article_driver.go treated ANY error from it as "feed not found,
// continue without feed_id". A transient DB error during feed lookup must
// not be silently reinterpreted as "no matching feed" — it must fail the
// save instead of persisting an article with a NULL feed_id that a working
// lookup would have resolved.
func TestAltDBRepository_SaveArticle_FeedLookupDBError_FailsFast(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

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

	// A real DB failure (not "no rows") during feed lookup.
	mock.ExpectQuery(`SELECT id FROM feeds WHERE website_url = \$1`).
		WithArgs("https://example.com/article").
		WillReturnError(errors.New("connection reset by peer"))

	// The upsert must NOT be attempted — SaveArticle must fail fast instead
	// of silently continuing with feed_id=NULL.
	_, err = repo.SaveArticle(ctx, "https://example.com/article", "Example Title", "<p>content</p>")
	require.Error(t, err)
	require.ErrorContains(t, err, "connection reset by peer")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_SaveArticle_ValidationFailures(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	t.Run("nil repository", func(t *testing.T) {
		var repo *ArticleRepository
		_, err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.Equal(t, "database connection not available", err.Error())
	})

	t.Run("nil pool", func(t *testing.T) {
		repo := &ArticleRepository{}
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
		repo := &ArticleRepository{pool: mock}
		_, err := repo.SaveArticle(ctxWithUser, "   ", "title", "content")
		require.Error(t, err)
		require.Equal(t, "article url cannot be empty", err.Error())
	})

	t.Run("empty content", func(t *testing.T) {
		repo := &ArticleRepository{pool: mock}
		_, err := repo.SaveArticle(ctxWithUser, "https://example.com", "title", "   ")
		require.Error(t, err)
		require.Equal(t, "article content cannot be empty", err.Error())
	})

	t.Run("missing user context", func(t *testing.T) {
		repo := &ArticleRepository{pool: mock}
		_, err := repo.SaveArticle(ctx, "https://example.com", "title", "content")
		require.Error(t, err)
		require.ErrorContains(t, err, "user context required")
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
