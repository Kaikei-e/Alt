package alt_db

import (
	"alt/domain"
	"context"
	"net/url"
)

func (r *AltDBRepository) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	// Scope to user when context is available
	user, userErr := domain.GetUserFromContext(ctx)
	if userErr == nil {
		query := `
			SELECT
				s.summary_japanese
			FROM
				article_summaries s
			JOIN
				articles a
			ON
				s.article_id = a.id
			WHERE
				a.url = $1 AND s.user_id = $2
			LIMIT 1
		`

		var summary domain.FeedSummary
		err := r.pool.QueryRow(ctx, query, feedURL.String(), user.UserID).Scan(&summary.Summary)
		if err != nil {
			return nil, err
		}
		return &summary, nil
	}

	// Fallback without user_id for internal API calls
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

// FetchArticleSummaryByArticleID fetches an article summary by article ID.
// Scopes to the authenticated user when user context is available.
func (r *AltDBRepository) FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error) {
	// Scope to user when context is available
	user, userErr := domain.GetUserFromContext(ctx)
	if userErr == nil {
		query := `
			SELECT
				summary_japanese
			FROM
				article_summaries
			WHERE
				article_id = $1 AND user_id = $2
			LIMIT 1
		`

		var summary domain.FeedSummary
		err := r.pool.QueryRow(ctx, query, articleID, user.UserID).Scan(&summary.Summary)
		if err != nil {
			return nil, err
		}
		return &summary, nil
	}

	// Fallback without user_id for internal API calls
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
