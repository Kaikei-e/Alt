package alt_db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchArticleByID_ScopesByUserContextWhenPresent(t *testing.T) {
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

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleByIDWithUserQuery)).
		WithArgs(articleID, userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url", "feed_id"}).
			AddRow(articleID, "Title", "Content", "https://example.com/a", "f1"))

	article, err := repo.FetchArticleByID(ctx, articleID)
	require.NoError(t, err)
	require.NotNil(t, article)
	assert.Equal(t, articleID, article.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticleByID_UnscopedWhenNoUserInContext(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	articleID := "22222222-2222-4222-8222-222222222222"

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleByIDQuery)).
		WithArgs(articleID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url", "feed_id"}).
			AddRow(articleID, "Title", "Content", "https://example.com/a", "f1"))

	article, err := repo.FetchArticleByID(context.Background(), articleID)
	require.NoError(t, err)
	require.NotNil(t, article)
	assert.Equal(t, articleID, article.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSummaryAndTagSetVersionByID_ScopesByUserContextWhenPresent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	svID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	tsvID := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	now := time.Now().UTC()

	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "u@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   now,
		ExpiresAt: now.Add(time.Hour),
	})

	summaryRepo := &SummaryRepository{pool: mock}
	tagRepo := &TagRepository{pool: mock}

	mock.ExpectQuery(`FROM summary_versions\s+WHERE summary_version_id = \$1 AND user_id = \$2`).
		WithArgs(svID, userID).
		WillReturnRows(pgxmock.NewRows([]string{
			"summary_version_id", "article_id", "user_id", "generated_at",
			"model", "prompt_version", "input_hash", "quality_score",
			"summary_text", "superseded_by",
		}).AddRow(svID, articleID, userID, now, "m", "v1", "h", (*float64)(nil), "summary", nil))

	mock.ExpectQuery(`FROM tag_set_versions\s+WHERE tag_set_version_id = \$1 AND user_id = \$2`).
		WithArgs(tsvID, userID).
		WillReturnRows(pgxmock.NewRows([]string{
			"tag_set_version_id", "article_id", "user_id", "generated_at",
			"generator", "input_hash", "tags_json", "superseded_by",
		}).AddRow(tsvID, articleID, userID, now, "g", "h", []byte(`["t1"]`), nil))

	sv, err := summaryRepo.GetSummaryVersionByID(ctx, svID)
	require.NoError(t, err)
	assert.Equal(t, svID, sv.SummaryVersionID)

	tsv, err := tagRepo.GetTagSetVersionByID(ctx, tsvID)
	require.NoError(t, err)
	assert.Equal(t, tsvID, tsv.TagSetVersionID)

	require.NoError(t, mock.ExpectationsWereMet())
}
