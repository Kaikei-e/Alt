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

	// Filter feed_tags first to bound the self-join scope, then rank pairs by
	// Ochiai association so high-frequency hub pairs do not crowd out strongly
	// associated smaller-topic pairs within the 2,000-pair cutoff.
	query := `
		WITH target_tags AS (
			SELECT id, tag_name FROM feed_tags WHERE tag_name = ANY($1)
		),
		tag_counts AS (
			SELECT tt.tag_name, COUNT(DISTINCT at.article_id) AS article_count
			FROM target_tags tt
			INNER JOIN article_tags at ON at.feed_tag_id = tt.id
			GROUP BY tt.tag_name
		),
		pair_counts AS (
			SELECT tt1.tag_name AS tag_a,
			       tt2.tag_name AS tag_b,
			       COUNT(DISTINCT at1.article_id) AS shared_count
			FROM article_tags at1
			INNER JOIN target_tags tt1 ON at1.feed_tag_id = tt1.id
			INNER JOIN article_tags at2
			  ON at1.article_id = at2.article_id AND tt1.tag_name < tt2.tag_name
			INNER JOIN target_tags tt2 ON at2.feed_tag_id = tt2.id
			GROUP BY tt1.tag_name, tt2.tag_name
			HAVING COUNT(DISTINCT at1.article_id) >= 2
		)
		SELECT pc.tag_a, pc.tag_b, pc.shared_count
		FROM pair_counts pc
		INNER JOIN tag_counts tc1 ON pc.tag_a = tc1.tag_name
		INNER JOIN tag_counts tc2 ON pc.tag_b = tc2.tag_name
		ORDER BY pc.shared_count::float8 / SQRT(tc1.article_count::float8 * tc2.article_count::float8) DESC,
		         pc.shared_count DESC,
		         pc.tag_a ASC,
		         pc.tag_b ASC
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
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "error iterating tag cooccurrences", "error", err)
		return nil, errors.New("error scanning tag cooccurrence")
	}

	return items, nil
}
