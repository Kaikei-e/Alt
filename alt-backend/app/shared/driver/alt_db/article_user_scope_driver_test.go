package alt_db

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The *ForUser driver variants exist because alt-data-hub serves these reads
// and writes over Connect-RPC (ADR-000954 Wave 3 batch 2, catalog §2.B /
// §2.C), where there is no Go request context carrying a signed-in user. The
// context-reading originals stay as thin wrappers so the in-process callers
// that have not moved yet keep working, and so that the tenant predicate is
// written once.

func TestSaveArticleForUser_UpsertsAndEnqueuesInOneTransaction(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	articleID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	content := strings.Repeat("x", 120)

	mock.ExpectQuery(`SELECT id FROM feeds WHERE website_url = \$1`).
		WithArgs("https://example.com/a").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Title", content, "https://example.com/a", userID, nil).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow(articleID, true))
	mock.ExpectExec(regexp.QuoteMeta(insertOutboxQuery)).
		WithArgs("ARTICLE_UPSERT", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	gotID, created, err := repo.SaveArticleForUser(context.Background(), "https://example.com/a", "Title", content, userID)
	require.NoError(t, err)
	assert.Equal(t, articleID.String(), gotID)
	assert.True(t, created, "xmax = 0 means this call inserted the row")
	require.NoError(t, mock.ExpectationsWereMet())
}

// A caller with no user is refused rather than defaulted. articles is keyed by
// (url, user_id); writing the zero UUID would create a row nobody can read
// back through the tenant-scoped queries.
func TestSaveArticleForUser_RejectsZeroUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	_, _, err = repo.SaveArticleForUser(context.Background(), "https://example.com/a", "Title", strings.Repeat("x", 120), uuid.Nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user id")
	require.NoError(t, mock.ExpectationsWereMet())
}

// SaveArticle keeps reading the context, so the callers that still hold a
// request context do not change shape.
func TestSaveArticle_DelegatesToForUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.New()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "a@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	content := strings.Repeat("x", 120)
	articleID := uuid.New()

	mock.ExpectQuery(`SELECT id FROM feeds WHERE website_url = \$1`).
		WithArgs("https://example.com/a").
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(upsertArticleQuery)).
		WithArgs("Title", content, "https://example.com/a", userID, nil).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow(articleID, false))
	mock.ExpectExec(regexp.QuoteMeta(insertOutboxQuery)).
		WithArgs("ARTICLE_UPSERT", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	gotID, err := repo.SaveArticle(ctx, "https://example.com/a", "Title", content)
	require.NoError(t, err)
	assert.Equal(t, articleID.String(), gotID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticleByURLForUser(t *testing.T) {
	userID := uuid.MustParse("33333333-3333-4333-8333-333333333333")

	tests := []struct {
		name        string
		userID      *uuid.UUID
		wantQuery   string
		wantArgs    []interface{}
		description string
	}{
		{
			name:        "scoped when a user is given",
			userID:      &userID,
			wantQuery:   fetchArticleByURLWithUserQuery,
			wantArgs:    []interface{}{"https://example.com/a", userID},
			description: "the tenant predicate must survive the move to an explicit argument",
		},
		{
			name:        "unscoped when no user is given",
			userID:      nil,
			wantQuery:   fetchArticleByURLQuery,
			wantArgs:    []interface{}{"https://example.com/a"},
			description: "the anonymous path the driver already had stays reachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()

			repo := &ArticleRepository{pool: mock}
			mock.ExpectQuery(regexp.QuoteMeta(tt.wantQuery)).
				WithArgs(tt.wantArgs...).
				WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url", "feed_id"}).
					AddRow("a1", "T", "C", "https://example.com/a", "f1"))

			article, err := repo.FetchArticleByURLForUser(context.Background(), "https://example.com/a", tt.userID)
			require.NoError(t, err, tt.description)
			require.NotNil(t, article)
			assert.Equal(t, "a1", article.ID)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// A URL nobody archived is (nil, nil): the caller fetches the page on that
// answer, and an error would make it give up instead.
func TestFetchArticleByURLForUser_MissIsNilWithoutError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	mock.ExpectQuery(regexp.QuoteMeta(fetchArticleByURLQuery)).
		WithArgs("https://example.com/missing").
		WillReturnError(pgx.ErrNoRows)

	article, err := repo.FetchArticleByURLForUser(context.Background(), "https://example.com/missing", nil)
	require.NoError(t, err)
	assert.Nil(t, article)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesByURLsForUser_Scoped(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.New()
	urls := []string{"https://example.com/a", "https://example.com/b"}

	mock.ExpectQuery(regexp.QuoteMeta(fetchArticlesByURLsWithUserQuery)).
		WithArgs(urls, userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "content", "url", "feed_id"}).
			AddRow("a1", "T", "C", "https://example.com/a", "f1"))

	got, err := repo.FetchArticlesByURLsForUser(context.Background(), urls, &userID)
	require.NoError(t, err)
	require.Len(t, got, 1, "a URL with no archived article is absent, not present-and-empty")
	assert.Equal(t, "a1", got["https://example.com/a"].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesWithCursorForUser_RejectsZeroUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	_, err = repo.FetchArticlesWithCursorForUser(context.Background(), uuid.Nil, nil, 10)
	require.Error(t, err, "a per-user timeline with no user would page through everybody's articles")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticleIDsWithCursorForUser_RejectsZeroUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}

	_, err = repo.FetchArticleIDsWithCursorForUser(context.Background(), uuid.Nil, nil, 10)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchArticlesWithCursorForUser_PassesCursorAndLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &ArticleRepository{pool: mock}
	userID := uuid.New()
	cursor := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM articles`).
		WithArgs(userID, &cursor, 25).
		WillReturnRows(pgxmock.NewRows([]string{"id", "title", "url", "content", "published_at", "created_at", "tags"}).
			AddRow(uuid.New(), "T", "https://example.com/a", "C", cursor, cursor, []string{"go"}))

	articles, err := repo.FetchArticlesWithCursorForUser(context.Background(), userID, &cursor, 25)
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, []string{"go"}, articles[0].Tags)
	require.NoError(t, mock.ExpectationsWereMet())
}
