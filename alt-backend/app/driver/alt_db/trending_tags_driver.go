package alt_db

import (
	"alt/port/knowledge_home_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// buildTagArticleCountsQuery returns the SQL for fetching tag article counts.
// The query contains no business logic — just data retrieval with user scoping.
func buildTagArticleCountsQuery() string {
	return `SELECT ft.tag_name, COUNT(DISTINCT at.article_id) AS article_count
		FROM feed_tags ft
		JOIN article_tags at ON ft.id = at.feed_tag_id
		JOIN articles a ON at.article_id = a.id
		WHERE a.created_at >= $1
		  AND a.deleted_at IS NULL
		  AND a.user_id = $2
		GROUP BY ft.tag_name`
}

// FetchTagArticleCounts returns tag names with article counts since the given time for a user.
func (r *AltDBRepository) FetchTagArticleCounts(ctx context.Context, userID uuid.UUID, since time.Time) ([]knowledge_home_port.TagArticleCount, error) {
	if r.pool == nil {
		return nil, errors.New("database connection pool is nil")
	}

	rows, err := r.pool.Query(ctx, buildTagArticleCountsQuery(), since, userID)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch tag article counts", "error", err)
		return nil, errors.New("failed to fetch tag article counts")
	}
	defer rows.Close()

	var results []knowledge_home_port.TagArticleCount
	for rows.Next() {
		var item knowledge_home_port.TagArticleCount
		if err := rows.Scan(&item.TagName, &item.ArticleCount); err != nil {
			logger.SafeErrorContext(ctx, "failed to scan tag article count", "error", err)
			continue
		}
		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.New("failed to iterate tag article counts")
	}

	return results, nil
}
