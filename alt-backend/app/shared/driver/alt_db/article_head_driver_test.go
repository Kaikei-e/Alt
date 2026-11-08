package alt_db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CleanupExpiredArticleHeads becomes load-bearing for the 7-day OG image
// retention policy, so pin its behaviour: it deletes by created_at and reports
// the number of purged rows.
func TestArticleRepository_CleanupExpiredArticleHeads(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	mock.ExpectExec(`(?s)DELETE FROM article_heads.*created_at <`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("DELETE", 4))

	deleted, err := repo.CleanupExpiredArticleHeads(context.Background(), 7*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 4, deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDs_ScopesByUserContextWhenPresent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := "22222222-2222-4222-8222-222222222222"

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs([]string{articleID}, userID).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "og_image_url"}).
			AddRow(articleID, "https://cdn.example.com/og.png"))

	urls, err := repo.FetchOgImageURLsByArticleIDs(ctx, []string{articleID})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{articleID: "https://cdn.example.com/og.png"}, urls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDsForUser_ScopesByUserAndPreventsCrossUserRead(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	articleID := "22222222-2222-4222-8222-222222222222"

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs([]string{articleID}, userA).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "og_image_url"}).
			AddRow(articleID, "https://cdn.example.com/private-a.png"))

	urlsA, err := repo.FetchOgImageURLsByArticleIDsForUser(context.Background(), []string{articleID}, &userA)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{articleID: "https://cdn.example.com/private-a.png"}, urlsA)

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs([]string{articleID}, userB).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "og_image_url"}))

	urlsB, err := repo.FetchOgImageURLsByArticleIDsForUser(context.Background(), []string{articleID}, &userB)
	require.NoError(t, err)
	assert.Empty(t, urlsB)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDs_RejectsMissingUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	articleID := "22222222-2222-4222-8222-222222222222"

	urls, err := repo.FetchOgImageURLsByArticleIDs(context.Background(), []string{articleID})
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, urls)

	urls, err = repo.FetchOgImageURLsByArticleIDsForUser(context.Background(), []string{articleID}, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, urls)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLByArticleID_ScopesByUserContext(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	articleID := "22222222-2222-4222-8222-222222222222"

	ctxA := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID: userA, Email: "a@example.com", Role: domain.UserRoleUser, TenantID: userA, ExpiresAt: time.Now().Add(time.Hour),
	})
	ctxB := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID: userB, Email: "b@example.com", Role: domain.UserRoleUser, TenantID: userB, ExpiresAt: time.Now().Add(time.Hour),
	})

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs([]string{articleID}, userA).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "og_image_url"}).
			AddRow(articleID, "https://cdn.example.com/private-a.png"))

	urlA, err := repo.FetchOgImageURLByArticleID(ctxA, articleID)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example.com/private-a.png", urlA)

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs([]string{articleID}, userB).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "og_image_url"}))

	urlB, err := repo.FetchOgImageURLByArticleID(ctxB, articleID)
	require.NoError(t, err)
	assert.Empty(t, urlB)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDsForUser_ScanErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleIDs := []string{"art-1", "art-2"}

	rows := pgxmock.NewRows([]string{"article_id", "og_image_url"}).
		AddRow("art-1", "https://example.com/1.png").
		AddRow("art-2", struct{}{})

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs(articleIDs, userID).
		WillReturnRows(rows)

	urls, err := repo.FetchOgImageURLsByArticleIDsForUser(context.Background(), articleIDs, &userID)
	require.Error(t, err)
	assert.Nil(t, urls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArticleRepository_FetchOgImageURLsByArticleIDsForUser_RowsErrorReturnsError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleIDs := []string{"art-1"}

	rows := pgxmock.NewRows([]string{"article_id", "og_image_url"}).
		AddRow("art-1", "https://example.com/1.png").
		RowError(0, errors.New("cursor closed"))

	mock.ExpectQuery(regexp.QuoteMeta(fetchOgImageURLsByArticleIDsWithUserQuery)).
		WithArgs(articleIDs, userID).
		WillReturnRows(rows)

	urls, err := repo.FetchOgImageURLsByArticleIDsForUser(context.Background(), articleIDs, &userID)
	require.Error(t, err)
	assert.Nil(t, urls)
	require.NoError(t, mock.ExpectationsWereMet())
}
