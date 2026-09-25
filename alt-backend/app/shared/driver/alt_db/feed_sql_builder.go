package alt_db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ogImageSelectExpr resolves the OG image for a feed row in the read path,
// preferring the most specific answer available.
//
//  1. article_heads — the article page's canonical image, scraped when someone
//     opened the article. More stable than RSS dynamic/expiring URLs and the
//     main remedy for proxy 404s.
//  2. feed_og_images — the on-demand resolution, scraped when a reader brought
//     the card into view. This is the only source for the ~92% of image-less
//     feeds that have no articles row at all, so nothing above can cover them.
//  3. feeds.og_image_url — the RSS-derived reference.
//
// The image is only surfaced when the feed is within the 7-day copyright
// retention window; older feeds return NULL so the frontend renders a
// placeholder. Aliased as og_image_url so existing row scans are unchanged.
const ogImageSelectExpr = `CASE WHEN f.created_at >= NOW() - INTERVAL '7 days' THEN COALESCE(
		       (SELECT ah.og_image_url FROM article_heads ah
		          JOIN articles a2 ON a2.id = ah.article_id
		         WHERE a2.feed_id = f.id AND a2.deleted_at IS NULL
		           AND ah.og_image_url IS NOT NULL AND ah.og_image_url <> ''
		         ORDER BY a2.created_at DESC LIMIT 1),
		       (SELECT foi.og_image_url FROM feed_og_images foi
		         WHERE foi.feed_id = f.id AND foi.state = 'resolved'),
		       f.og_image_url) ELSE NULL END AS og_image_url`

// buildExcludeClauseMultiple builds a WHERE clause that excludes feeds matching
// any of the given feed_link_ids using PostgreSQL array comparison.
// Converts []uuid.UUID to []string for pgx encoding compatibility.
func buildExcludeClauseMultiple(args []any, excludeFeedLinkIDs []uuid.UUID) (string, []any) {
	if len(excludeFeedLinkIDs) == 0 {
		return "", args
	}
	strs := make([]string, len(excludeFeedLinkIDs))
	for i, id := range excludeFeedLinkIDs {
		strs[i] = id.String()
	}
	clause := fmt.Sprintf(`AND f.feed_link_id != ALL($%d::uuid[])`, len(args)+1)
	args = append(args, strs)
	return clause, args
}

// Builds the SQL query and arguments for fetching unread feeds with cursor pagination.
func buildUnreadFeedsCursorQuery(cursor *time.Time, excludeFeedLinkIDs []uuid.UUID, limit int, userID uuid.UUID) (string, []any) {
	var query string
	var args []any
	var excludeClause string

	if cursor == nil {
		args = []any{limit, userID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
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
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr, excludeClause)
	} else {
		args = []any{cursor, limit, userID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
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
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr, excludeClause)
	}

	return query, args
}

// Builds the SQL query and arguments for fetching all feeds with cursor pagination.
func buildAllFeedsCursorQuery(cursor *time.Time, excludeFeedLinkIDs []uuid.UUID, limit int, userID uuid.UUID) (string, []any) {
	var query string
	var args []any
	var excludeClause string

	if cursor == nil {
		args = []any{limit, userID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, ogImageSelectExpr, excludeClause)
	} else {
		args = []any{cursor, limit, userID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       %s
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $3
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, ogImageSelectExpr, excludeClause)
	}

	return query, args
}

// Builds the SQL query and arguments for fetching read feeds with cursor pagination.
func buildReadFeedsCursorQuery(cursor *time.Time, limit int, userID uuid.UUID) (string, []any) {
	var query string
	var args []any

	if cursor == nil {
		query = fmt.Sprintf(`
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
		args = []any{limit, userID}
	} else {
		query = fmt.Sprintf(`
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
		args = []any{cursor, limit, userID}
	}

	return query, args
}

// Builds the SQL query and arguments for fetching favorite feeds with cursor pagination.
func buildFavoriteFeedsCursorQuery(cursor *time.Time, limit int, userID uuid.UUID) (string, []any) {
	var query string
	var args []any

	if cursor == nil {
		query = fmt.Sprintf(`
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
		args = []any{limit, userID}
	} else {
		query = fmt.Sprintf(`
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
		args = []any{cursor, limit, userID}
	}

	return query, args
}
