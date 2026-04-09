package alt_db

import (
	"alt/domain"
	"alt/driver/models"
	"alt/utils/constants"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type FeedPageRow struct {
	FeedID      uuid.UUID
	Title       string
	Description string
	Link        string
	PubDate     time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArticleID   *string
	OgImageURL  *string
}

func (r *FeedRepository) GetSingleFeed(ctx context.Context) (*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 1
	`

	var feed models.Feed
	err := r.pool.QueryRow(ctx, query).Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching single feed", "error", err)
		return nil, errors.New("error fetching single feed")
	}

	return &feed, nil
}

func (r *FeedRepository) FetchFeedsList(ctx context.Context) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 10000
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching feeds list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *FeedRepository) FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching feeds list limit", "error", err)
		return nil, errors.New("error fetching feeds list limit")
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list offset", "error", err)
			return nil, errors.New("error scanning feeds list offset")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *FeedRepository) FetchFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query, constants.DefaultPageSize, constants.DefaultPageSize*page)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching feeds list page", "error", err)
		return nil, errors.New("error fetching feeds list page")
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *FeedRepository) FetchUnreadFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	// For now, keeping the original OFFSET-based implementation for backward compatibility
	// Consider migrating to cursor-based pagination (FetchUnreadFeedsListCursor) for better performance
	query := `
		SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
		FROM feeds f
		WHERE NOT EXISTS (
			SELECT 1
			FROM read_status rs
			WHERE rs.feed_id = f.id
			AND rs.user_id = $3
			AND rs.is_read = TRUE
		)
		ORDER BY f.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, constants.DefaultPageSize, constants.DefaultPageSize*page, user.UserID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching unread feeds list page", "error", err, "user_id", user.UserID)
		return nil, errors.New("error fetching feeds list page")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning unread feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

// buildExcludeClause builds a WHERE clause that excludes feeds matching
// a single feed_link_id. Delegates to buildExcludeClauseMultiple.
func buildExcludeClause(args []any, excludeFeedLinkID *uuid.UUID) (string, []any) {
	if excludeFeedLinkID == nil {
		return "", args
	}
	return buildExcludeClauseMultiple(args, []uuid.UUID{*excludeFeedLinkID})
}

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

func (r *FeedRepository) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchUnreadFeedsListCursor")
	defer span.End()

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	// Cursor-based pagination using created_at only
	// created_at is always populated (NOT NULL DEFAULT CURRENT_TIMESTAMP) and reliable
	// pub_date has many zero values (0001-01-01) and is not reliable for pagination
	// LEFT JOIN with articles table to get article_id if article exists
	var query string
	var args []interface{}

	var excludeClause string
	if cursor == nil {
		args = []interface{}{limit, user.UserID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       f.og_image_url
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
		`, excludeClause)
	} else {
		args = []interface{}{cursor, limit, user.UserID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       f.og_image_url
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
		`, excludeClause)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching unread feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt, &feed.ArticleID, &feed.OgImageURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning unread feeds with cursor", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(feeds)))
	return feeds, nil
}

// FetchAllFeedsListCursor retrieves all feeds (read + unread) using cursor-based pagination.
// Unlike FetchUnreadFeedsListCursor, this does not filter by read status but includes
// the read status via LEFT JOIN so the frontend can visually distinguish read/unread feeds.
func (r *FeedRepository) FetchAllFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchAllFeedsListCursor")
	defer span.End()

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	var excludeClause string
	if cursor == nil {
		args = []interface{}{limit, user.UserID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       f.og_image_url
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $1
		`, excludeClause)
	} else {
		args = []interface{}{cursor, limit, user.UserID}
		excludeClause, args = buildExcludeClauseMultiple(args, excludeFeedLinkIDs)
		query = fmt.Sprintf(`
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
			       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
			       COALESCE(rs.is_read, FALSE) AS is_read,
			       f.og_image_url
			FROM feeds f
			LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $3
			WHERE f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND f.created_at < $1
			%s
			ORDER BY f.created_at DESC, f.id DESC
			LIMIT $2
		`, excludeClause)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching all feeds with cursor", "error", err, "cursor", cursor)
		return nil, errors.New("error fetching feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt, &feed.ArticleID, &feed.IsRead, &feed.OgImageURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning all feeds with cursor", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(feeds)))
	return feeds, nil
}

// FetchReadFeedsListCursor retrieves read feeds using cursor-based pagination
// This method uses INNER JOIN with read_status table for better performance
func (r *FeedRepository) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchReadFeedsListCursor")
	defer span.End()

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $2
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $1
		`
		args = []interface{}{limit, user.UserID}
	} else {
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $3
			AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
			AND rs.read_at < $1
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $2
		`
		args = []interface{}{cursor, limit, user.UserID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching read feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching read feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning read feeds with cursor", "error", err)
			return nil, errors.New("error scanning read feeds list")
		}
		feeds = append(feeds, &feed)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(feeds)))
	return feeds, nil
}

func (r *FeedRepository) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		query = `
                       SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
                              (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
                              f.og_image_url
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $2
                       AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $2)
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $1
               `
		args = []interface{}{limit, user.UserID}
	} else {
		query = `
                       SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
                              (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
                              f.og_image_url
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $3 AND ff.created_at < $1
                       AND f.feed_link_id IN (SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $3)
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $2
               `
		args = []interface{}{cursor, limit, user.UserID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching favorite feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching favorite feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt, &feed.ArticleID, &feed.OgImageURL)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning favorite feeds with cursor", "error", err)
			return nil, errors.New("error scanning favorite feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *FeedRepository) FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*FeedPageRow, error) {
	query := `
		SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at,
		       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL
		        ORDER BY a.created_at DESC LIMIT 1) AS article_id,
		       f.og_image_url
		FROM feeds f
		WHERE f.feed_link_id = $1
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT 200
	`

	rows, err := r.pool.Query(ctx, query, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("query feeds by feed_link_id: %w", err)
	}
	defer rows.Close()

	result := make([]*FeedPageRow, 0)
	for rows.Next() {
		var row FeedPageRow
		if err := rows.Scan(&row.FeedID, &row.Title, &row.Description, &row.Link, &row.PubDate, &row.CreatedAt, &row.UpdatedAt, &row.ArticleID, &row.OgImageURL); err != nil {
			return nil, fmt.Errorf("scan feed page row: %w", err)
		}
		result = append(result, &row)
	}
	return result, rows.Err()
}

func (r *FeedRepository) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	query := `
		SELECT feed_id FROM read_status
		WHERE user_id = $1 AND feed_id = ANY($2::uuid[]) AND is_read = TRUE
	`

	feedIDStrings := make([]string, 0, len(feedIDs))
	for _, feedID := range feedIDs {
		feedIDStrings = append(feedIDStrings, feedID.String())
	}

	rows, err := r.pool.Query(ctx, query, userID, feedIDStrings)
	if err != nil {
		return nil, fmt.Errorf("query read feed ids: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool, len(feedIDs))
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			return nil, fmt.Errorf("scan read feed id: %w", err)
		}
		result[feedID] = true
	}
	return result, rows.Err()
}

// maxReadFeedIDs bounds the result set to prevent unbounded growth.
const maxReadFeedIDs = 10000

func (r *FeedRepository) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	query := `
		SELECT feed_id FROM read_status
		WHERE user_id = $1 AND is_read = TRUE
		ORDER BY read_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, userID, maxReadFeedIDs)
	if err != nil {
		return nil, fmt.Errorf("query all read feed ids: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool)
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			return nil, fmt.Errorf("scan read feed id: %w", err)
		}
		result[feedID] = true
	}
	return result, rows.Err()
}

func (r *FeedRepository) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $1`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user subscriptions: %w", err)
	}
	defer rows.Close()

	result := make([]uuid.UUID, 0)
	for rows.Next() {
		var feedLinkID uuid.UUID
		if err := rows.Scan(&feedLinkID); err != nil {
			return nil, fmt.Errorf("scan user subscription: %w", err)
		}
		result = append(result, feedLinkID)
	}
	return result, rows.Err()
}
