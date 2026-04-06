package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

// SearchTagsByPrefix searches for tags matching a prefix and returns them with article counts.
func (r *TagRepository) SearchTagsByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT ft.tag_name, COUNT(DISTINCT at.article_id) as article_count
		FROM feed_tags ft
		INNER JOIN article_tags at ON ft.id = at.feed_tag_id
		WHERE ft.tag_name ILIKE $1 || '%'
		GROUP BY ft.tag_name
		HAVING COUNT(DISTINCT at.article_id) > 0
		ORDER BY article_count DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, prefix, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error searching tags by prefix", "error", err, "prefix", prefix)
		return nil, errors.New("error searching tags by prefix")
	}
	defer rows.Close()

	var hits []domain.GlobalTagHit
	for rows.Next() {
		var hit domain.GlobalTagHit
		if err := rows.Scan(&hit.TagName, &hit.ArticleCount); err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning tag search result", "error", err)
			return nil, errors.New("error scanning tag search result")
		}
		hits = append(hits, hit)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "row iteration error in tag search", "error", err)
		return nil, errors.New("error iterating tag search results")
	}

	return hits, nil
}
