package alt_db

import (
	"alt/domain"
	"context"
	"net/url"
)

func (r *AltDBRepository) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	query := `
		SELECT
			summary
		FROM
			article_summaries
		LEFT JOIN
			articles
		ON
			article_summaries.article_id = articles.id
		WHERE
			articles.url = $1
	`

	var summary domain.FeedSummary
	err := r.pool.QueryRow(ctx, query, feedURL).Scan(&summary.Summary)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}
