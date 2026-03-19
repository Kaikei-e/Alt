// ABOUTME: Tests for article repository content preservation guard
// ABOUTME: Verifies "longer content wins" logic prevents data quality regression

package repository

import (
	"context"
	"database/sql"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pre-processor-sidecar/models"
)

func newTestRepo(t *testing.T) (*PostgreSQLArticleRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	logger := slog.Default()
	repo := &PostgreSQLArticleRepository{db: db, logger: logger}
	return repo, mock
}

func newTestArticle() *models.Article {
	now := time.Now()
	published := now.Add(-time.Hour)
	return &models.Article{
		ID:             uuid.New(),
		InoreaderID:    "tag:google.com,2005:reader/item/00000000deadbeef",
		SubscriptionID: uuid.New(),
		ArticleURL:     "https://example.com/article-1",
		Title:          "Test Article",
		Author:         "Author",
		PublishedAt:    &published,
		FetchedAt:      now,
		Processed:      false,
		Content:        "<p>Short content</p>",
		ContentLength:  20,
		ContentType:    "html",
	}
}

var (
	selectContentLengthQuery = regexp.QuoteMeta(
		"SELECT COALESCE(content_length, 0) FROM inoreader_articles WHERE inoreader_id = $1",
	)
	upsertFullQuery = regexp.QuoteMeta(
		`INSERT INTO inoreader_articles (
			id, inoreader_id, subscription_id, article_url, title, author,
			published_at, fetched_at, processed, content, content_length, content_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (inoreader_id)
		DO UPDATE SET
			subscription_id = EXCLUDED.subscription_id,
			article_url = EXCLUDED.article_url,
			title = EXCLUDED.title,
			author = EXCLUDED.author,
			published_at = EXCLUDED.published_at,
			fetched_at = EXCLUDED.fetched_at,
			content = EXCLUDED.content,
			content_length = EXCLUDED.content_length,
			content_type = EXCLUDED.content_type
		RETURNING (xmax = 0) AS was_inserted`,
	)
	updateMetadataOnlyQuery = regexp.QuoteMeta(
		`UPDATE inoreader_articles SET
			subscription_id = $2,
			article_url = $3,
			title = $4,
			author = $5,
			published_at = $6,
			fetched_at = $7
		WHERE inoreader_id = $1`,
	)
)

func TestCreateWithResult_Insert_NewArticle(t *testing.T) {
	repo, mock := newTestRepo(t)
	article := newTestArticle()

	// No existing row → sql.ErrNoRows
	mock.ExpectQuery(selectContentLengthQuery).
		WithArgs(article.InoreaderID).
		WillReturnError(sql.ErrNoRows)

	// Full upsert (INSERT)
	mock.ExpectQuery(upsertFullQuery).
		WithArgs(
			article.ID, article.InoreaderID, article.SubscriptionID,
			article.ArticleURL, article.Title, article.Author,
			article.PublishedAt, article.FetchedAt, article.Processed,
			article.Content, article.ContentLength, article.ContentType,
		).
		WillReturnRows(sqlmock.NewRows([]string{"was_inserted"}).AddRow(true))

	result, err := repo.CreateWithResult(context.Background(), article)
	require.NoError(t, err)
	assert.True(t, result.WasInserted)
	assert.False(t, result.ContentKept)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateWithResult_Update_LongerContent(t *testing.T) {
	repo, mock := newTestRepo(t)
	article := newTestArticle()
	article.ContentLength = 500 // incoming is longer

	// Existing row has shorter content
	mock.ExpectQuery(selectContentLengthQuery).
		WithArgs(article.InoreaderID).
		WillReturnRows(sqlmock.NewRows([]string{"content_length"}).AddRow(100))

	// Full upsert (UPDATE with new content)
	mock.ExpectQuery(upsertFullQuery).
		WithArgs(
			article.ID, article.InoreaderID, article.SubscriptionID,
			article.ArticleURL, article.Title, article.Author,
			article.PublishedAt, article.FetchedAt, article.Processed,
			article.Content, article.ContentLength, article.ContentType,
		).
		WillReturnRows(sqlmock.NewRows([]string{"was_inserted"}).AddRow(false))

	result, err := repo.CreateWithResult(context.Background(), article)
	require.NoError(t, err)
	assert.False(t, result.WasInserted)
	assert.False(t, result.ContentKept)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateWithResult_Update_ShorterContent_PreservesExisting(t *testing.T) {
	repo, mock := newTestRepo(t)
	article := newTestArticle()
	article.ContentLength = 50 // incoming is shorter

	// Existing row has longer content
	mock.ExpectQuery(selectContentLengthQuery).
		WithArgs(article.InoreaderID).
		WillReturnRows(sqlmock.NewRows([]string{"content_length"}).AddRow(500))

	// Metadata-only update (content preserved)
	mock.ExpectExec(updateMetadataOnlyQuery).
		WithArgs(
			article.InoreaderID, article.SubscriptionID,
			article.ArticleURL, article.Title, article.Author,
			article.PublishedAt, article.FetchedAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := repo.CreateWithResult(context.Background(), article)
	require.NoError(t, err)
	assert.False(t, result.WasInserted)
	assert.True(t, result.ContentKept)
	require.NoError(t, mock.ExpectationsWereMet())
}
