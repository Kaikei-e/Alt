package alt_db

import (
	"context"
	"fmt"
)

// OgBackfillCandidate identifies an article whose OG image is missing and should
// be scraped from its page.
type OgBackfillCandidate struct {
	ArticleID string
	URL       string
}

// FetchFeedsMissingOgImage returns recent articles (within the 7-day retention
// window) whose feed has no RSS-derived og_image_url and which have no scraped
// article_heads og:image yet. These are the work-list for the OG image backfill
// job. The limit bounds the per-run scraping budget.
func (r *FeedRepository) FetchFeedsMissingOgImage(ctx context.Context, limit int) ([]OgBackfillCandidate, error) {
	const query = `
		SELECT a.id::text, a.url
		FROM articles a
		JOIN feeds f ON f.id = a.feed_id
		WHERE a.deleted_at IS NULL
		  AND f.created_at >= NOW() - INTERVAL '7 days'
		  AND (f.og_image_url IS NULL OR f.og_image_url = '')
		  AND NOT EXISTS (
		      SELECT 1 FROM article_heads ah
		      WHERE ah.article_id = a.id
		        AND ah.og_image_url IS NOT NULL
		        AND ah.og_image_url <> ''
		  )
		ORDER BY f.created_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query feeds missing og image: %w", err)
	}
	defer rows.Close()

	candidates := make([]OgBackfillCandidate, 0, limit)
	for rows.Next() {
		var c OgBackfillCandidate
		if err := rows.Scan(&c.ArticleID, &c.URL); err != nil {
			return nil, fmt.Errorf("scan og backfill candidate: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}
