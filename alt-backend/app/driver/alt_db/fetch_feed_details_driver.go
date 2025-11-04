package alt_db

import (
	"alt/domain"
	"context"
	"net/url"
)

func (r *AltDBRepository) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	query := `
		SELECT
			s.summary_japanese
		FROM
			article_summaries s
		LEFT JOIN
			articles a
		ON
			s.article_id = a.id
		WHERE
			a.url = $1
		LIMIT 1
	`

	var summary domain.FeedSummary
	err := r.pool.QueryRow(ctx, query, feedURL.String()).Scan(&summary.Summary)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}

// FetchArticleSummaryByArticleID fetches an article summary by article ID
func (r *AltDBRepository) FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error) {
	query := `
		SELECT
			summary_japanese
		FROM
			article_summaries
		WHERE
			article_id = $1
		LIMIT 1
	`

	var summary domain.FeedSummary
	err := r.pool.QueryRow(ctx, query, articleID).Scan(&summary.Summary)
	if err != nil {
		return nil, err
	}

	return &summary, nil
}
