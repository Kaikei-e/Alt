package alt_db

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUnreadFeedsCursorQuery_NilCursor(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	limit := 20

	t.Run("without exclusions", func(t *testing.T) {
		gotSQL, gotArgs := buildUnreadFeedsCursorQuery(nil, nil, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $2
				AND rs.is_read = TRUE
			)
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 2)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
	})

	t.Run("with exclusions", func(t *testing.T) {
		exID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
		gotSQL, gotArgs := buildUnreadFeedsCursorQuery(nil, []uuid.UUID{exID}, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $2
				AND rs.is_read = TRUE
			)
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			AND f.feed_link_id != ALL($3::uuid[])
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
		assert.Equal(t, []string{exID.String()}, gotArgs[2])
	})
}

func TestBuildUnreadFeedsCursorQuery_WithCursor(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cursor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	limit := 30

	t.Run("without exclusions", func(t *testing.T) {
		gotSQL, gotArgs := buildUnreadFeedsCursorQuery(&cursor, nil, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $3
				AND rs.is_read = TRUE
			)
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
	})

	t.Run("with exclusions", func(t *testing.T) {
		exID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
		gotSQL, gotArgs := buildUnreadFeedsCursorQuery(&cursor, []uuid.UUID{exID}, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $3
				AND rs.is_read = TRUE
			)
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			AND f.feed_link_id != ALL($4::uuid[])
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 4)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
		assert.Equal(t, []string{exID.String()}, gotArgs[3])
	})
}

func TestBuildAllFeedsCursorQuery_NilCursor(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	limit := 25

	t.Run("without exclusions", func(t *testing.T) {
		gotSQL, gotArgs := buildAllFeedsCursorQuery(nil, nil, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 2)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
	})

	t.Run("with exclusions", func(t *testing.T) {
		exID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
		gotSQL, gotArgs := buildAllFeedsCursorQuery(nil, []uuid.UUID{exID}, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			AND f.feed_link_id != ALL($3::uuid[])
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
		assert.Equal(t, []string{exID.String()}, gotArgs[2])
	})
}

func TestBuildAllFeedsCursorQuery_WithCursor(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	cursor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	limit := 35

	t.Run("without exclusions", func(t *testing.T) {
		gotSQL, gotArgs := buildAllFeedsCursorQuery(&cursor, nil, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $3
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
	})

	t.Run("with exclusions", func(t *testing.T) {
		exID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
		gotSQL, gotArgs := buildAllFeedsCursorQuery(&cursor, []uuid.UUID{exID}, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $3
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			AND f.feed_link_id != ALL($4::uuid[])
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 4)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
		assert.Equal(t, []string{exID.String()}, gotArgs[3])
	})
}

func TestBuildReadFeedsCursorQuery(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	limit := 15

	t.Run("nil cursor", func(t *testing.T) {
		gotSQL, gotArgs := buildReadFeedsCursorQuery(nil, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $2
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 2)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
	})

	t.Run("with cursor", func(t *testing.T) {
		cursor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
		gotSQL, gotArgs := buildReadFeedsCursorQuery(&cursor, limit, userID)

		wantSQL := fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       %s
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $3
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND rs.read_at < $1
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
	})
}

func TestBuildFavoriteFeedsCursorQuery(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	limit := 10

	t.Run("nil cursor", func(t *testing.T) {
		gotSQL, gotArgs := buildFavoriteFeedsCursorQuery(nil, limit, userID)

		wantSQL := fmt.Sprintf(`
                       SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
                              (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
                              %s
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $2
                       AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $1
               `, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 2)
		assert.Equal(t, limit, gotArgs[0])
		assert.Equal(t, userID, gotArgs[1])
	})

	t.Run("with cursor", func(t *testing.T) {
		cursor := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
		gotSQL, gotArgs := buildFavoriteFeedsCursorQuery(&cursor, limit, userID)

		wantSQL := fmt.Sprintf(`
                       SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
                              (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
                              %s
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $3 AND ff.created_at < $1
                       AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $2
               `, ogImageSelectExpr)

		assert.Equal(t, wantSQL, gotSQL)
		require.Len(t, gotArgs, 3)
		assert.Equal(t, &cursor, gotArgs[0])
		assert.Equal(t, limit, gotArgs[1])
		assert.Equal(t, userID, gotArgs[2])
	})
}
