package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"fmt"
)

// FetchInoreaderSummariesByURLs retrieves inoreader article summaries for the given URLs
func (r *AltDBRepository) FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error) {
	if len(urls) == 0 {
		logger.Logger.Info("No URLs provided for inoreader summaries fetch")
		return []*models.InoreaderSummary{}, nil
	}

	logger.Logger.Info("Fetching inoreader summaries", "url_count", len(urls), "urls", urls)

	query := `
		SELECT
			ia.article_url,
			ia.title,
			ia.author,
			ia.content,
			ia.content_type,
			ia.published_at,
			ia.fetched_at,
			ia.inoreader_id
		FROM inoreader_articles ia
		WHERE ia.article_url = ANY($1)
		AND ia.content IS NOT NULL
		AND ia.content_length > 0
		ORDER BY ia.published_at DESC
	`

	rows, err := r.pool.Query(ctx, query, urls)
	if err != nil {
		logger.Logger.Error("Failed to query inoreader summaries", "error", err, "url_count", len(urls))
		return nil, fmt.Errorf("failed to query inoreader summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*models.InoreaderSummary
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
			logger.Logger.Error("Failed to scan inoreader summary row", "error", err)
			return nil, fmt.Errorf("failed to scan inoreader summary: %w", err)
		}
		summaries = append(summaries, &summary)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error iterating inoreader summary rows", "error", err)
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	logger.Logger.Info("Successfully fetched inoreader summaries",
		"requested_count", len(urls),
		"matched_count", len(summaries))

	return summaries, nil
}
