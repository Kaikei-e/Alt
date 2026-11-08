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

func TestFetchArticleTags_ScopesByUserContextWhenPresent(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := "22222222-2222-4222-8222-222222222222"
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleTagsWithUserQuery)).
		WithArgs(articleID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tag_name", "created_at"}).
			AddRow("tag-1", "golang", now))

	tags, err := repo.FetchArticleTags(ctx, articleID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "golang", tags[0].TagName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticleTagsForUser_ScopesByUserAndPreventsCrossUserRead(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	userA := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	userB := uuid.MustParse("99999999-9999-4999-8999-999999999999")
	articleID := "22222222-2222-4222-8222-222222222222"
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleTagsWithUserQuery)).
		WithArgs(articleID, userA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tag_name", "created_at"}).
			AddRow("tag-1", "private-tag", now))

	tagsA, err := repo.FetchArticleTagsForUser(context.Background(), articleID, &userA)
	require.NoError(t, err)
	require.Len(t, tagsA, 1)
	assert.Equal(t, "private-tag", tagsA[0].TagName)

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleTagsWithUserQuery)).
		WithArgs(articleID, userB).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tag_name", "created_at"}))

	tagsB, err := repo.FetchArticleTagsForUser(context.Background(), articleID, &userB)
	require.NoError(t, err)
	assert.Empty(t, tagsB)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticleTags_RejectsMissingUser(t *testing.T) {
	logger.InitLogger()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &TagRepository{pool: mock}
	articleID := "22222222-2222-4222-8222-222222222222"

	tags, err := repo.FetchArticleTags(context.Background(), articleID)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, tags)

	tags, err = repo.FetchArticleTagsForUser(context.Background(), articleID, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "user")
	assert.Nil(t, tags)

	require.NoError(t, mock.ExpectationsWereMet())
}
