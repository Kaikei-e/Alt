package alt_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"alt/domain"
	"alt/utils/logger"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchArticlesByTag_ScopesByUserContextWhenPresent(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	tagID := "22222222-2222-4222-8222-222222222222"
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	cols := []string{"id", "title", "url", "created_at", "feed_id", "feed_title"}

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagFirstPageWithUserQuery)).
		WithArgs(tagID, userID, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a1", "Title 1", "https://example.com/1", now, "f1", "Feed 1"))

	articles, err := repo.FetchArticlesByTag(ctx, tagID, nil, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "a1", articles[0].ID)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagPaginatedWithUserQuery)).
		WithArgs(tagID, userID, &cursor, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a2", "Title 2", "https://example.com/2", now, "f1", "Feed 1"))

	articles, err = repo.FetchArticlesByTag(ctx, tagID, &cursor, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "a2", articles[0].ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByTagForUser_ScopesByUserAndPreventsCrossUserRead(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	tagID := "22222222-2222-4222-8222-222222222222"
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	cols := []string{"id", "title", "url", "created_at", "feed_id", "feed_title"}

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagFirstPageWithUserQuery)).
		WithArgs(tagID, userA, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a1", "Private A", "https://example.com/a", now, "f1", "Feed A"))

	articlesA, err := repo.FetchArticlesByTagForUser(context.Background(), tagID, &userA, nil, 20)
	require.NoError(t, err)
	require.Len(t, articlesA, 1)
	assert.Equal(t, "Private A", articlesA[0].Title)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagFirstPageWithUserQuery)).
		WithArgs(tagID, userB, 20).
		WillReturnRows(pgxmock.NewRows(cols))

	articlesB, err := repo.FetchArticlesByTagForUser(context.Background(), tagID, &userB, nil, 20)
	require.NoError(t, err)
	assert.Empty(t, articlesB)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByTag_RejectsMissingUser(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	tagID := "22222222-2222-4222-8222-222222222222"

	articles, err := repo.FetchArticlesByTag(context.Background(), tagID, nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	articles, err = repo.FetchArticlesByTagForUser(context.Background(), tagID, nil, nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	ctxWithUser := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New()})
	articles, err = repo.FetchArticlesByTagForUser(ctxWithUser, tagID, nil, nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByTagName_ScopesByUserContextWhenPresent(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	cursor := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	cols := []string{"id", "title", "url", "created_at", "feed_id", "feed_title"}

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagNameFirstPageWithUserQuery)).
		WithArgs("golang", userID, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a1", "Title 1", "https://example.com/1", now, "f1", "Feed 1"))

	articles, err := repo.FetchArticlesByTagName(ctx, "golang", nil, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "a1", articles[0].ID)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagNamePaginatedWithUserQuery)).
		WithArgs("golang", userID, &cursor, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a2", "Title 2", "https://example.com/2", now, "f1", "Feed 1"))

	articles, err = repo.FetchArticlesByTagName(ctx, "golang", &cursor, 20)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "a2", articles[0].ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByTagNameForUser_ScopesByUserAndPreventsCrossUserRead(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	cols := []string{"id", "title", "url", "created_at", "feed_id", "feed_title"}

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagNameFirstPageWithUserQuery)).
		WithArgs("golang", userA, 20).
		WillReturnRows(pgxmock.NewRows(cols).
			AddRow("a1", "Private A", "https://example.com/a", now, "f1", "Feed A"))

	articlesA, err := repo.FetchArticlesByTagNameForUser(context.Background(), "golang", &userA, nil, 20)
	require.NoError(t, err)
	require.Len(t, articlesA, 1)
	assert.Equal(t, "Private A", articlesA[0].Title)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByTagNameFirstPageWithUserQuery)).
		WithArgs("golang", userB, 20).
		WillReturnRows(pgxmock.NewRows(cols))

	articlesB, err := repo.FetchArticlesByTagNameForUser(context.Background(), "golang", &userB, nil, 20)
	require.NoError(t, err)
	assert.Empty(t, articlesB)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByTagName_RejectsMissingUser(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}

	articles, err := repo.FetchArticlesByTagName(context.Background(), "golang", nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	articles, err = repo.FetchArticlesByTagNameForUser(context.Background(), "golang", nil, nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	ctxWithUser := domain.SetUserContext(context.Background(), &domain.UserContext{UserID: uuid.New()})
	articles, err = repo.FetchArticlesByTagNameForUser(ctxWithUser, "golang", nil, nil, 20)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, articles)

	require.NoError(t, mock.ExpectationsWereMet())
}
