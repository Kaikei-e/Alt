package alt_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagRepository_FetchTagCloudForUser_ScopesToUsersArticles(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	mock.ExpectQuery(`JOIN articles a ON at\.article_id = a\.id[\s\S]*a\.user_id = \$1[\s\S]*a\.deleted_at IS NULL`).
		WithArgs(testTagUserID, 25).
		WillReturnRows(pgxmock.NewRows([]string{"tag_name", "article_count"}).AddRow("golang", 4))

	items, err := repo.FetchTagCloudForUser(context.Background(), testTagUserID, 25)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "golang", items[0].TagName)
	assert.Equal(t, 4, items[0].ArticleCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTagRepository_FetchTagCloudForUser_RejectsNilUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	_, err = repo.FetchTagCloudForUser(context.Background(), uuid.Nil, 25)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "no query may be issued without a user id")
}

func TestTagRepository_FetchTagCloud_StaysInstanceWide(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := NewTagRepository(mock)

	mock.ExpectQuery(`FROM feed_tags ft\s+INNER JOIN article_tags at ON ft\.id = at\.feed_tag_id\s+GROUP BY`).
		WithArgs(25).
		WillReturnRows(pgxmock.NewRows([]string{"tag_name", "article_count"}))

	_, err = repo.FetchTagCloud(context.Background(), 25)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
