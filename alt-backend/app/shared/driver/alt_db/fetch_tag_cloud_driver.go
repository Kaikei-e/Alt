package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchTagCloud fetches tag names with their article counts, ordered by count descending.
func (r *TagRepository) FetchTagCloud(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	query := `
		SELECT ft.tag_name, COUNT(DISTINCT at.article_id) as article_count
		FROM feed_tags ft
		INNER JOIN article_tags at ON ft.id = at.feed_tag_id
		GROUP BY ft.tag_name
		HAVING COUNT(DISTINCT at.article_id) > 0
		ORDER BY article_count DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching tag cloud", "error", err)
		return nil, errors.New("error fetching tag cloud")
	}
	defer rows.Close()

	var items []*domain.TagCloudItem
	for rows.Next() {
		var item domain.TagCloudItem
		err := rows.Scan(&item.TagName, &item.ArticleCount)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning tag cloud item", "error", err)
			return nil, errors.New("error scanning tag cloud item")
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating tag cloud", "error", err)
		return nil, errors.New("error scanning tag cloud item")
	}

	return items, nil
}

// FetchTagCloudForUser fetches tag names with their article counts over one
// user's articles, ordered by count descending.
func (r *TagRepository) FetchTagCloudForUser(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.TagCloudItem, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
		return nil, errors.New("user id is required")
	}

	query := `
		SELECT ft.tag_name, COUNT(DISTINCT at.article_id) as article_count
		FROM feed_tags ft
		INNER JOIN article_tags at ON ft.id = at.feed_tag_id
		INNER JOIN articles a ON at.article_id = a.id
		WHERE a.user_id = $1
		  AND a.deleted_at IS NULL
		GROUP BY ft.tag_name
		HAVING COUNT(DISTINCT at.article_id) > 0
		ORDER BY article_count DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching user tag cloud", "error", err)
		return nil, errors.New("error fetching tag cloud")
	}
	defer rows.Close()

	var items []*domain.TagCloudItem
	for rows.Next() {
		var item domain.TagCloudItem
		if err := rows.Scan(&item.TagName, &item.ArticleCount); err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning user tag cloud item", "error", err)
			return nil, errors.New("error scanning tag cloud item")
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating user tag cloud", "error", err)
		return nil, errors.New("error scanning tag cloud item")
	}

	return items, nil
}
