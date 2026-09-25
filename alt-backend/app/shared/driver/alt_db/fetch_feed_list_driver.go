package alt_db

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils/constants"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (r *FeedRepository) GetSingleFeed(ctx context.Context) (*models.Feed, error) {
	query := `
		SELECT id, title, description, website_url, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 1
	`

	var feed models.Feed
	err := r.pool.QueryRow(ctx, query).Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching single feed", "error", err)
		return nil, errors.New("error fetching single feed")
	}

	return &feed, nil
}

func (r *FeedRepository) FetchFeedsList(ctx context.Context) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, website_url, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT 10000
	`

	var feeds []*models.Feed
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error fetching feeds list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating feeds list", "error", err)
		return nil, errors.New("error scanning feeds list")
	}

	return feeds, nil
}

func (r *FeedRepository) FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, website_url, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1
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
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list offset", "error", err)
			return nil, errors.New("error scanning feeds list offset")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating feeds list limit", "error", err)
		return nil, errors.New("error scanning feeds list offset")
	}

	return feeds, nil
}

func (r *FeedRepository) FetchFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	query := `
		SELECT id, title, description, website_url, pub_date, created_at, updated_at FROM feeds ORDER BY created_at DESC LIMIT $1 OFFSET $2
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
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating feeds list page", "error", err)
		return nil, errors.New("error scanning feeds list page")
	}

	return feeds, nil
}

// FetchUnreadFeedsListPage reads the signed-in user out of the request context.
//
// The *ForUser variants below exist because alt-data-hub serves these reads
// over Connect-RPC (ADR-000954 Wave 3 batch 3, capability catalog §2.H), where
// there is no Go request context carrying a person: the peer certificate says
// "alt-backend" and nothing about whose feeds are being listed. The
// context-reading wrappers stay so the in-process callers that have not moved
// keep working, and so the tenant predicate is written once.
func (r *FeedRepository) FetchUnreadFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}
	return r.FetchUnreadFeedsListPageForUser(ctx, user.UserID, page)
}

func (r *FeedRepository) FetchUnreadFeedsListPageForUser(ctx context.Context, userID uuid.UUID, page int) ([]*models.Feed, error) {
	// For now, keeping the original OFFSET-based implementation for backward compatibility
	// Consider migrating to cursor-based pagination (FetchUnreadFeedsListCursor) for better performance
	query := `
		SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at
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

	rows, err := r.pool.Query(ctx, query, constants.DefaultPageSize, constants.DefaultPageSize*page, userID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching unread feeds list page", "error", err, "user_id", userID)
		return nil, errors.New("error fetching feeds list page")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := rows.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning unread feeds list page", "error", err)
			return nil, errors.New("error scanning feeds list page")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating unread feeds list page", "error", err)
		return nil, errors.New("error scanning feeds list page")
	}

	return feeds, nil
}
