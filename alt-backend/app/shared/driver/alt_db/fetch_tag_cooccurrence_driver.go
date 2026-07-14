package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

// FetchTagCooccurrences fetches pairs of tags that share articles.
// Only returns pairs where both tag names are in the provided list and share at least 2 articles.
func (r *TagRepository) FetchTagCooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error) {
	if len(tagNames) == 0 {
		return nil, nil
	}

	// CTE-based query: filter feed_tags first to reduce self-join scope.
	// This avoids scanning the full article_tags table when the target tag set is small.
	query := `
		WITH target_tags AS (
			SELECT id, tag_name FROM feed_tags WHERE tag_name = ANY($1)
		)
		SELECT tt1.tag_name AS tag_a, tt2.tag_name AS tag_b,
		       COUNT(DISTINCT at1.article_id) AS shared_count
		FROM article_tags at1
		INNER JOIN target_tags tt1 ON at1.feed_tag_id = tt1.id
		INNER JOIN article_tags at2
		  ON at1.article_id = at2.article_id AND at1.feed_tag_id < at2.feed_tag_id
		INNER JOIN target_tags tt2 ON at2.feed_tag_id = tt2.id
		GROUP BY tt1.tag_name, tt2.tag_name
		HAVING COUNT(DISTINCT at1.article_id) >= 2
		ORDER BY shared_count DESC
		LIMIT 2000
	`

	rows, err := r.pool.Query(ctx, query, tagNames)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "error fetching tag cooccurrences", "error", err)
		return nil, errors.New("error fetching tag cooccurrences")
	}
	defer rows.Close()

	var items []*domain.TagCooccurrence
	for rows.Next() {
		var item domain.TagCooccurrence
		err := rows.Scan(&item.TagNameA, &item.TagNameB, &item.SharedCount)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "error scanning tag cooccurrence", "error", err)
			return nil, errors.New("error scanning tag cooccurrence")
		}
		items = append(items, &item)
	}

	return items, nil
}
