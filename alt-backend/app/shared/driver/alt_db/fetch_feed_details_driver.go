package alt_db

import (
	"alt/domain"
	"context"
	"net/url"

	"github.com/google/uuid"
)

// FetchFeedSummary reads the signed-in user out of the request context.
//
// The *ForUser variants take the tenant explicitly because alt-data-hub serves
// these over Connect-RPC (ADR-000954 Wave 3 batch 3, capability catalog §2.H),
// where the context carries a peer certificate and not a person. A nil userID
// selects the unscoped query — the fallback these methods have always had for
// service-to-service callers — rather than defaulting to some user.
func (r *FeedRepository) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	user, userErr := domain.GetUserFromContext(ctx)
	if userErr != nil {
		return r.FetchFeedSummaryForUser(ctx, feedURL, nil)
	}
	return r.FetchFeedSummaryForUser(ctx, feedURL, &user.UserID)
}

func (r *FeedRepository) FetchFeedSummaryForUser(ctx context.Context, feedURL *url.URL, userID *uuid.UUID) (*domain.FeedSummary, error) {
	if userID != nil {
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
		err := r.pool.QueryRow(ctx, query, feedURL.String(), *userID).Scan(&summary.Summary)
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
func (r *FeedRepository) FetchArticleSummaryByArticleID(ctx context.Context, articleID string) (*domain.FeedSummary, error) {
	user, userErr := domain.GetUserFromContext(ctx)
	if userErr != nil {
		return r.FetchArticleSummaryByArticleIDForUser(ctx, articleID, nil)
	}
	return r.FetchArticleSummaryByArticleIDForUser(ctx, articleID, &user.UserID)
}

func (r *FeedRepository) FetchArticleSummaryByArticleIDForUser(ctx context.Context, articleID string, userID *uuid.UUID) (*domain.FeedSummary, error) {
	if userID != nil {
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
		err := r.pool.QueryRow(ctx, query, articleID, *userID).Scan(&summary.Summary)
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
