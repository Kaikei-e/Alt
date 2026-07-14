package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
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

	return items, nil
}
