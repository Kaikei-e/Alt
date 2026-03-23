package alt_db

import (
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

var (
	selectArticleContentLengthQuery = regexp.QuoteMeta(
		"SELECT COALESCE(OCTET_LENGTH(content), 0) FROM articles WHERE url = $1 AND user_id = $2 AND deleted_at IS NULL",
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
)

func TestAltDBRepository_CreateArticleInternal_Success(t *testing.T) {
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

// TestCreateArticleInternal_JapaneseContent_PreservesLonger verifies that the
// OCTET_LENGTH-based guard correctly protects Japanese content. With the old
// LENGTH (character count), a 800-char Japanese article (~2400 bytes) would
// appear "longer" than a 1000-char existing article (LENGTH=1000), bypassing
// the guard. OCTET_LENGTH returns bytes, matching Go's len().
func TestCreateArticleInternal_JapaneseContent_PreservesLonger(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	publishedAt := time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC)

	// Simulate: existing article has 3000 bytes of Japanese content (≈1000 chars)
	// New content is 800 Japanese chars = ~2400 bytes.
	// With LENGTH (chars): 1000 > 2400 → false → OVERWRITE (bug)
	// With OCTET_LENGTH (bytes): 3000 > 2400 → true → PRESERVE (correct)
	jaContent := strings.Repeat("あ", 800) // 800 chars × 3 bytes = 2400 bytes

	mock.ExpectBegin()
	mock.ExpectQuery(selectArticleContentLengthQuery).
		WithArgs("https://example.com/ja-article", "00000000-0000-4000-a000-000000000001").
		WillReturnRows(pgxmock.NewRows([]string{"content_length"}).AddRow(3000))
	mock.ExpectQuery(metadataOnlyArticleUpsertQuery).
		WithArgs("日本語タイトル", jaContent, "https://example.com/ja-article", "feed-1", "00000000-0000-4000-a000-000000000001", publishedAt).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created"}).AddRow("22222222-2222-4222-a222-222222222222", false))
	mock.ExpectCommit()

	articleID, created, err := repo.CreateArticleInternal(t.Context(), CreateArticleParams{
		Title:       "日本語タイトル",
		URL:         "https://example.com/ja-article",
		Content:     jaContent,
		FeedID:      "feed-1",
		UserID:      "00000000-0000-4000-a000-000000000001",
		PublishedAt: publishedAt,
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, "22222222-2222-4222-a222-222222222222", articleID)
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
