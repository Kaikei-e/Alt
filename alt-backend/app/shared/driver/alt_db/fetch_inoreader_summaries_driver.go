package alt_db

import (
	"alt/orchestrator/driver/models"
	"alt/utils/logger"
	"context"
	"fmt"
)

// FetchInoreaderSummariesByURLs retrieves persisted article summaries/content
// for the given URLs from the articles table (where pre-processor syncs
// Inoreader articles after ADR-000246 / ADR-000397 dropped inoreader_articles
// from alt-db).
func (r *FeedRepository) FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error) {
	if len(urls) == 0 {
		logger.Logger.InfoContext(ctx, "No URLs provided for inoreader summaries fetch")
		return []*models.InoreaderSummary{}, nil
	}

	if r.pool == nil {
		return nil, fmt.Errorf("database connection not available")
	}

	logger.Logger.InfoContext(ctx, "Fetching inoreader summaries", "url_count", len(urls), "urls", urls)

	query := `
		SELECT
			a.url AS article_url,
			a.title,
			NULL::text AS author,
			a.content,
			'html'::text AS content_type,
			COALESCE(a.published_at, a.created_at) AS published_at,
			a.created_at AS fetched_at,
			a.id::text AS inoreader_id
		FROM articles a
		WHERE a.url = ANY($1)
		AND a.deleted_at IS NULL
		AND a.content IS NOT NULL
		AND length(a.content) > 0
		ORDER BY COALESCE(a.published_at, a.created_at) DESC
	`

	rows, err := r.pool.Query(ctx, query, urls)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to query inoreader summaries", "error", err, "url_count", len(urls))
		return nil, fmt.Errorf("failed to query inoreader summaries: %w", err)
	}
	defer rows.Close()

	summaries := make([]*models.InoreaderSummary, 0)
	for rows.Next() {
		var summary models.InoreaderSummary
		err := rows.Scan(
			&summary.ArticleURL,
			&summary.Title,
			&summary.Author,
			&summary.Content,
			&summary.ContentType,
			&summary.PublishedAt,
			&summary.FetchedAt,
			&summary.InoreaderID,
		)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to scan inoreader summary row", "error", err)
			return nil, fmt.Errorf("failed to scan inoreader summary: %w", err)
		}
		summaries = append(summaries, &summary)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "Error iterating inoreader summary rows", "error", err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Successfully fetched inoreader summaries",
		"requested_count", len(urls),
		"matched_count", len(summaries))

	return summaries, nil
}
