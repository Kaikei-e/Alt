package alt_db

import (
	"database/sql"
	"regexp"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

var (
	selectArticleContentLengthQuery = regexp.QuoteMeta(
		"SELECT COALESCE(LENGTH(content), 0) FROM articles WHERE url = $1 AND user_id = $2 AND deleted_at IS NULL",
	)
	fullArticleUpsertQuery = regexp.QuoteMeta(`
		INSERT INTO articles (title, content, url, feed_id, user_id, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url, user_id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id),
			published_at = EXCLUDED.published_at
		RETURNING id, (xmax = 0) AS created
	`)
	metadataOnlyArticleUpsertQuery = regexp.QuoteMeta(`
		INSERT INTO articles (title, content, url, feed_id, user_id, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url, user_id) DO UPDATE SET
			title = EXCLUDED.title,
			feed_id = COALESCE(EXCLUDED.feed_id, articles.feed_id),
			published_at = EXCLUDED.published_at
		RETURNING id, (xmax = 0) AS created
	`)
	knowledgeEventInsertQuery = regexp.QuoteMeta(`INSERT INTO knowledge_events
		(event_id, occurred_at, tenant_id, user_id, actor_type, actor_id,
		 event_type, aggregate_type, aggregate_id, correlation_id, causation_id,
		 dedupe_key, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (dedupe_key) DO NOTHING`)
)

func expectKnowledgeEventInsert(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(knowledgeEventInsertQuery).
		WithArgs(
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
}

func TestAltDBRepository_CreateArticleInternal_AppendsKnowledgeEvent(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	publishedAt := time.Date(2026, 3, 18, 8, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	// Content length check: no existing row
	mock.ExpectQuery(selectArticleContentLengthQuery).
		WithArgs("https://example.com/article", "00000000-0000-4000-a000-000000000001").
		WillReturnError(sql.ErrNoRows)
	// Full upsert (new article)
	mock.ExpectQuery(fullArticleUpsertQuery).
		WithArgs("Title", "Body", "https://example.com/article", "feed-1", "00000000-0000-4000-a000-000000000001", publishedAt).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow("11111111-1111-4111-a111-111111111111", true))
	expectKnowledgeEventInsert(mock)
	mock.ExpectCommit()

	articleID, created, err := repo.CreateArticleInternal(t.Context(), CreateArticleParams{
		Title:       "Title",
		URL:         "https://example.com/article",
		Content:     "Body",
		FeedID:      "feed-1",
		UserID:      "00000000-0000-4000-a000-000000000001",
		PublishedAt: publishedAt,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "11111111-1111-4111-a111-111111111111", articleID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateArticleInternal_ShorterContent_PreservesExisting(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	publishedAt := time.Date(2026, 3, 18, 8, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	// Existing article has longer content (500 chars)
	mock.ExpectQuery(selectArticleContentLengthQuery).
		WithArgs("https://example.com/article", "00000000-0000-4000-a000-000000000001").
		WillReturnRows(pgxmock.NewRows([]string{"content_length"}).AddRow(500))
	// Metadata-only upsert (content excluded from DO UPDATE SET)
	mock.ExpectQuery(metadataOnlyArticleUpsertQuery).
		WithArgs("Title", "Short", "https://example.com/article", "feed-1", "00000000-0000-4000-a000-000000000001", publishedAt).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow("11111111-1111-4111-a111-111111111111", false))
	expectKnowledgeEventInsert(mock)
	mock.ExpectCommit()

	articleID, created, err := repo.CreateArticleInternal(t.Context(), CreateArticleParams{
		Title:       "Title",
		URL:         "https://example.com/article",
		Content:     "Short", // 5 chars < 500 existing
		FeedID:      "feed-1",
		UserID:      "00000000-0000-4000-a000-000000000001",
		PublishedAt: publishedAt,
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "11111111-1111-4111-a111-111111111111", articleID)
	require.NoError(t, mock.ExpectationsWereMet())
}
