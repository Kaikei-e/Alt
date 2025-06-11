package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
)

func (r *AltDBRepository) GetSingleFeed(ctx context.Context) (*models.Feed, error) {
	query := `
		SELECT id, title, description, link, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 1
	`

	var feed models.Feed
	err := r.db.QueryRow(ctx, query).Scan(&feed.ID, &feed.Title, &feed.Description, &feed.Link, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
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
	rows, err := r.db.Query(ctx, query)
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
	rows, err := r.db.Query(ctx, query, limit)
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
	rows, err := r.db.Query(ctx, query, pageSize, pageSize*page)
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
	const pageSize = 10

	// Alternative query using LEFT JOIN - might be more efficient than NOT EXISTS
	query := `
		SELECT f.id, f.title, f.description, f.link, f.pub_date, f.created_at, f.updated_at
		FROM feeds f
		LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.is_read = TRUE
		WHERE rs.feed_id IS NULL
		ORDER BY f.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, pageSize, pageSize*page)
	if err != nil {
		logger.Logger.Error("error fetching unread feeds list page", "error", err)
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
