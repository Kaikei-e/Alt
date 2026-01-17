package alt_db

import (
	"alt/domain"
	"context"
	"log/slog"
	"strings"
	"time"
)

// SearchFeedsByTitle searches feeds by title using PostgreSQL ILIKE for case-insensitive search
// NOTE: This function is currently NOT used in production. The application uses SearchFeedMeilisearchUsecase instead.
// Searches feed titles only (adding article content search would cause severe performance issues with LIKE queries).
// Filters feeds by user_id to ensure multi-tenant isolation
func (a *AltDBRepository) SearchFeedsByTitle(ctx context.Context, query string, userID string) ([]*domain.FeedItem, error) {
	// Return empty result for empty query
	if strings.TrimSpace(query) == "" {
		slog.InfoContext(ctx, "empty query provided, returning empty results")
		return []*domain.FeedItem{}, nil
	}

	// Convert query to lowercase for case-insensitive search
	searchPattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"

	queryString := `
		SELECT DISTINCT f.id, f.title, f.description, f.link, f.pub_date, f.created_at
		FROM feeds f
		INNER JOIN articles a ON f.id = a.feed_id
		WHERE a.user_id = $1
		AND LOWER(f.title) LIKE $2
		ORDER BY f.pub_date DESC
		LIMIT 50
	`

	slog.InfoContext(ctx, "searching feeds by title",
		"query", query,
		"user_id", userID)

	rows, err := a.pool.Query(ctx, queryString, userID, searchPattern)
	if err != nil {
		slog.ErrorContext(ctx, "failed to search feeds by title",
			"error", err,
			"query", query,
			"user_id", userID)
		return nil, err
	}
	defer rows.Close()

	feeds := []*domain.FeedItem{}

	for rows.Next() {
		var feed domain.FeedItem
		var feedID string
		var pubDate *time.Time
		var createdAt time.Time

		err := rows.Scan(
			&feedID,
			&feed.Title,
			&feed.Description,
			&feed.Link,
			&pubDate,
			&createdAt,
		)
		if err != nil {
			slog.ErrorContext(ctx, "failed to scan feed row",
				"error", err)
			return nil, err
		}

		// Set the published field using the pub_date from database
		if pubDate != nil {
			feed.Published = pubDate.Format(time.RFC3339)
			feed.PublishedParsed = *pubDate
		} else {
			// Use created_at as fallback if pub_date is null
			feed.Published = createdAt.Format(time.RFC3339)
			feed.PublishedParsed = createdAt
		}

		feeds = append(feeds, &feed)
	}

	if err := rows.Err(); err != nil {
		slog.ErrorContext(ctx, "error iterating feed rows",
			"error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "feed search completed",
		"query", query,
		"user_id", userID,
		"results_count", len(feeds))

	return feeds, nil
}
