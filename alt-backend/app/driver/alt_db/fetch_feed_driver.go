package alt_db

import (
	"alt/domain"
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"time"
)

func (r *AltDBRepository) GetSingleFeed(ctx context.Context) (*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 1
	`

	var feed models.Feed
	err := r.pool.QueryRow(ctx, query).Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
	if err != nil {
		logger.Logger.Error("error fetching single feed", "error", err)
		return nil, errors.New("error fetching single feed")
	}

	return &feed, nil
}

func (r *AltDBRepository) FetchFeedsList(ctx context.Context) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC
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
			logger.Logger.Error("error scanning feeds list", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *AltDBRepository) FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		logger.Logger.Error("error fetching feeds list limit", "error", err)
		return nil, errors.New("error fetching feeds list limit")
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning feeds list offset", "error", err)
			return nil, errors.New("error scanning feeds list offset")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *AltDBRepository) FetchFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	const pageSize = 10

	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query, pageSize, pageSize*page)
	if err != nil {
		logger.Logger.Error("error fetching feeds list page", "error", err)
		return nil, errors.New("error fetching feeds list page")
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *AltDBRepository) FetchUnreadFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.Error("user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	const pageSize = 10

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

	rows, err := r.pool.Query(ctx, query, pageSize, pageSize*page, user.UserID)
	if err != nil {
		logger.Logger.Error("error fetching unread feeds list page", "error", err, "user_id", user.UserID)
		return nil, errors.New("error fetching feeds list page")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning unread feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *AltDBRepository) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.Error("user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	// Cursor-based pagination for better performance
	// Uses pub_date as cursor to show latest feeds first (falls back to created_at if pub_date is NULL)
	var query string
	var args []interface{}

	if cursor == nil {
		// First page - no cursor
		// Use pub_date if it's not NULL and not zero value (0001-01-01), otherwise use created_at
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $2
				AND rs.is_read = TRUE
			)
			ORDER BY CASE WHEN f.pub_date IS NOT NULL AND f.pub_date > '1970-01-01'::timestamp THEN f.pub_date ELSE f.created_at END DESC, f.id DESC
			LIMIT $1
		`
		args = []interface{}{limit, user.UserID}
	} else {
		// Subsequent pages - use cursor
		// Use pub_date if it's not NULL and not zero value (0001-01-01), otherwise use created_at
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			WHERE NOT EXISTS (
				SELECT 1
				FROM read_status rs
				WHERE rs.feed_id = f.id
				AND rs.user_id = $3
				AND rs.is_read = TRUE
			)
			AND CASE WHEN f.pub_date IS NOT NULL AND f.pub_date > '1970-01-01'::timestamp THEN f.pub_date ELSE f.created_at END < $1
			ORDER BY CASE WHEN f.pub_date IS NOT NULL AND f.pub_date > '1970-01-01'::timestamp THEN f.pub_date ELSE f.created_at END DESC, f.id DESC
			LIMIT $2
		`
		args = []interface{}{cursor, limit, user.UserID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.Error("error fetching unread feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning unread feeds with cursor", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

// FetchReadFeedsListCursor retrieves read feeds using cursor-based pagination
// This method uses INNER JOIN with read_status table for better performance
func (r *AltDBRepository) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.Error("user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		// Initial fetch: INNER JOIN for performance optimization
		// Order by read_at to show most recently read feeds first
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $2
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $1
		`
		args = []interface{}{limit, user.UserID}
	} else {
		// Subsequent pages: cursor-based pagination using read_at
		query = `
			SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
			FROM feeds f
			INNER JOIN read_status rs ON rs.feed_id = f.id
			WHERE rs.is_read = TRUE
			AND rs.user_id = $3
			AND rs.read_at < $1
			ORDER BY rs.read_at DESC, f.id DESC
			LIMIT $2
		`
		args = []interface{}{cursor, limit, user.UserID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.Error("error fetching read feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching read feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning read feeds with cursor", "error", err)
			return nil, errors.New("error scanning read feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}

func (r *AltDBRepository) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.Error("user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	var query string
	var args []interface{}

	if cursor == nil {
		query = `
                       SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $2
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $1
               `
		args = []interface{}{limit, user.UserID}
	} else {
		// Fixed: Use proper cursor-based pagination that handles edge cases
		// Order by ff.created_at since that's what we're using for the cursor
		query = `
                       SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
                       FROM feeds f
                       INNER JOIN favorite_feeds ff ON ff.feed_id = f.id
                       WHERE ff.user_id = $3 AND ff.created_at < $1
                       ORDER BY ff.created_at DESC, f.id DESC
                       LIMIT $2
               `
		args = []interface{}{cursor, limit, user.UserID}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.Error("error fetching favorite feeds with cursor", "error", err, "cursor", cursor, "user_id", user.UserID)
		return nil, errors.New("error fetching favorite feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("error scanning favorite feeds with cursor", "error", err)
			return nil, errors.New("error scanning favorite feeds list")
		}
		feeds = append(feeds, &feed)
	}

	return feeds, nil
}
