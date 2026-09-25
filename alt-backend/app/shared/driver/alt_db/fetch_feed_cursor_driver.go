package alt_db

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Scans row columns into a models.Feed destination struct.
func scanFeedPageRow(scanner rowScanner, feed *models.Feed, includeIsRead bool) error {
	if includeIsRead {
		return scanner.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt, &feed.ArticleID, &feed.IsRead, &feed.OgImageURL)
	}
	return scanner.Scan(&feed.ID, &feed.Title, &feed.Description, &feed.WebsiteURL, &feed.PubDate, &feed.CreatedAt, &feed.UpdatedAt, &feed.ArticleID, &feed.OgImageURL)
}

func (r *FeedRepository) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}
	return r.FetchUnreadFeedsListCursorForUser(ctx, user.UserID, cursor, limit, excludeFeedLinkIDs)
}

func (r *FeedRepository) FetchUnreadFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchUnreadFeedsListCursorForUser")
	defer span.End()

	// Cursor-based pagination using created_at only
	// created_at is always populated (NOT NULL DEFAULT CURRENT_TIMESTAMP) and reliable
	// pub_date has many zero values (0001-01-01) and is not reliable for pagination
	// LEFT JOIN with articles table to get article_id if article exists
	query, args := buildUnreadFeedsCursorQuery(cursor, excludeFeedLinkIDs, limit, userID)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching unread feeds with cursor", "error", err, "cursor", cursor, "user_id", userID)
		return nil, errors.New("error fetching feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := scanFeedPageRow(rows, &feed, false)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning unread feeds with cursor", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating unread feeds with cursor", "error", err)
		return nil, errors.New("error scanning feeds list")
	}

	span.SetAttributes(attribute.Int("db.row_count", len(feeds)))
	return feeds, nil
}

// FetchAllFeedsListCursor retrieves all feeds (read + unread) using cursor-based pagination.
// Unlike FetchUnreadFeedsListCursor, this does not filter by read status but includes
// the read status via LEFT JOIN so the frontend can visually distinguish read/unread feeds.
func (r *FeedRepository) FetchAllFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}
	return r.FetchAllFeedsListCursorForUser(ctx, user.UserID, cursor, limit, excludeFeedLinkIDs)
}

func (r *FeedRepository) FetchAllFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchAllFeedsListCursorForUser")
	defer span.End()

	query, args := buildAllFeedsCursorQuery(cursor, excludeFeedLinkIDs, limit, userID)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching all feeds with cursor", "error", err, "cursor", cursor)
		return nil, errors.New("error fetching feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := scanFeedPageRow(rows, &feed, true)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning all feeds with cursor", "error", err)
			return nil, errors.New("error scanning feeds list")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating all feeds with cursor", "error", err)
		return nil, errors.New("error scanning feeds list")
	}

	span.SetAttributes(attribute.Int("db.row_count", len(feeds)))
	return feeds, nil
}

// FetchReadFeedsListCursor retrieves read feeds using cursor-based pagination
// This method uses INNER JOIN with read_status table for better performance
func (r *FeedRepository) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}
	return r.FetchReadFeedsListCursorForUser(ctx, user.UserID, cursor, limit)
}

func (r *FeedRepository) FetchReadFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*models.Feed, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.FetchReadFeedsListCursorForUser")
	defer span.End()

	query, args := buildReadFeedsCursorQuery(cursor, limit, userID)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching read feeds with cursor", "error", err, "cursor", cursor, "user_id", userID)
		return nil, errors.New("error fetching read feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := scanFeedPageRow(rows, &feed, false)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning read feeds with cursor", "error", err)
			return nil, errors.New("error scanning read feeds list")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating read feeds with cursor", "error", err)
		return nil, errors.New("error scanning read feeds list")
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
	return r.FetchFavoriteFeedsListCursorForUser(ctx, user.UserID, cursor, limit)
}

func (r *FeedRepository) FetchFavoriteFeedsListCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*models.Feed, error) {
	query, args := buildFavoriteFeedsCursorQuery(cursor, limit, userID)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching favorite feeds with cursor", "error", err, "cursor", cursor, "user_id", userID)
		return nil, errors.New("error fetching favorite feeds list")
	}
	defer rows.Close()

	var feeds []*models.Feed
	for rows.Next() {
		var feed models.Feed
		err := scanFeedPageRow(rows, &feed, false)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning favorite feeds with cursor", "error", err)
			return nil, errors.New("error scanning favorite feeds list")
		}
		feeds = append(feeds, &feed)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating favorite feeds with cursor", "error", err)
		return nil, errors.New("error scanning favorite feeds list")
	}

	return feeds, nil
}
