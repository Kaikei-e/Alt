package alt_db

import (
	"context"
	"fmt"
	"time"
)

// BatchArticleTagRow is the driver-layer representation of a single
// (article_id, tag) row returned by BatchGetTagsByArticleIDs.
type BatchArticleTagRow struct {
	ArticleID  string
	TagName    string
	Confidence float32
	UpdatedAt  time.Time
}

const batchGetTagsByArticleIDsQuery = `
	SELECT
		a.id::text AS article_id,
		ft.tag_name,
		ft.confidence,
		COALESCE(ft.updated_at, ft.created_at) AS updated_at
	FROM articles a
	INNER JOIN article_tags at ON a.id = at.article_id
	INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
	WHERE a.id = ANY($1::uuid[])
	ORDER BY a.id, ft.confidence DESC
`

// BatchGetTagsByArticleIDs returns every (article_id, tag) pair for the
// supplied article ids joined through article_tags and feed_tags.
// Empty input yields an empty slice without touching the pool.
func (r *TagRepository) BatchGetTagsByArticleIDs(ctx context.Context, articleIDs []string) ([]BatchArticleTagRow, error) {
	if len(articleIDs) == 0 {
		return nil, nil
	}
	if r.pool == nil {
		return nil, fmt.Errorf("BatchGetTagsByArticleIDs: pool is nil")
	}

	rows, err := r.pool.Query(ctx, batchGetTagsByArticleIDsQuery, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("BatchGetTagsByArticleIDs query: %w", err)
	}
	defer rows.Close()

	var out []BatchArticleTagRow
	for rows.Next() {
		var row BatchArticleTagRow
		if err := rows.Scan(&row.ArticleID, &row.TagName, &row.Confidence, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("BatchGetTagsByArticleIDs scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("BatchGetTagsByArticleIDs rows: %w", err)
	}

	return out, nil
}
