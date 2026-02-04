package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

// FetchArticleTags retrieves tags associated with a specific article.
func (r *AltDBRepository) FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT ft.id, ft.tag_name, ft.created_at
		FROM feed_tags ft
		INNER JOIN article_tags at ON ft.id = at.feed_tag_id
		WHERE at.article_id = $1
		ORDER BY ft.created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, articleID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching article tags", "error", err, "articleID", articleID)
		return nil, errors.New("error fetching article tags")
	}
	defer rows.Close()

	var tags []*domain.FeedTag
	for rows.Next() {
		var tag domain.FeedTag
		err := rows.Scan(&tag.ID, &tag.TagName, &tag.CreatedAt)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning article tag", "error", err)
			return nil, errors.New("error scanning article tags")
		}
		tags = append(tags, &tag)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "row iteration error", "error", err)
		return nil, errors.New("error iterating article tags")
	}

	logger.Logger.InfoContext(ctx, "fetched article tags from database", "articleID", articleID, "count", len(tags))
	return tags, nil
}
