package driver

import (
	"context"
	"net/url"
	"pre-processor/logger"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func GetSourceURLs(lastCreatedAt *time.Time, lastID string, ctx context.Context, db *pgxpool.Pool) ([]url.URL, *time.Time, string, error) {
	var urls []url.URL
	var finalCreatedAt *time.Time
	var finalID string
	limit := 40

	err := retryDBOperation(ctx, func() error {
		var query string
		var args []interface{}

		if lastCreatedAt == nil || lastCreatedAt.IsZero() {
			// First query - no cursor constraint
			query = `
				SELECT f.link, f.created_at, f.id
				FROM   feeds f
				LEFT   JOIN articles a ON f.link = a.url
				WHERE  a.url IS NULL
				AND    f.link NOT LIKE '%.mp3'
				ORDER  BY f.created_at DESC, f.id DESC
				LIMIT  $1
			`
			args = []interface{}{limit}
		} else {
			// Subsequent queries - use efficient keyset pagination
			query = `
				SELECT f.link, f.created_at, f.id
				FROM   feeds f
				WHERE  f.link NOT LIKE '%.mp3'
				AND    (f.created_at, f.id) < ($1, $2)
				AND    NOT EXISTS ( SELECT 1
				                    FROM   articles a
				                    WHERE  a.url = f.link
				                    LIMIT  1 )
				ORDER  BY f.created_at DESC, f.id DESC
				LIMIT  $3
			`
			args = []interface{}{*lastCreatedAt, lastID, limit}
		}

		rows, err := db.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		urls = nil // Reset urls slice for retry
		for rows.Next() {
			var u string
			var createdAt time.Time
			var id string

			err = rows.Scan(&u, &createdAt, &id)
			if err != nil {
				return err
			}

			ul, err := convertToURL(u)
			if err != nil {
				logger.Logger.Error("Failed to convert URL", "error", err)
				continue // Skip invalid URLs but don't fail the whole operation
			}

			urls = append(urls, ul)
			// Keep track of the last item for cursor
			finalCreatedAt = &createdAt
			finalID = id
		}

		return rows.Err()
	}, "GetSourceURLs")

	if err != nil {
		logger.Logger.Error("Failed to get source URLs", "error", err)
		return nil, nil, "", err
	}

	// Add diagnostic logging when no URLs found
	if len(urls) == 0 {
		// Check total feeds and processed feeds for debugging
		var totalFeeds, processedFeeds int
		db.QueryRow(ctx, "SELECT COUNT(*) FROM feeds WHERE link NOT LIKE '%.mp3'").Scan(&totalFeeds)
		db.QueryRow(ctx, "SELECT COUNT(DISTINCT a.url) FROM articles a INNER JOIN feeds f ON a.url = f.link WHERE f.link NOT LIKE '%.mp3'").Scan(&processedFeeds)

		logger.Logger.Info("No URLs found for processing",
			"has_cursor", lastCreatedAt != nil,
			"total_feeds", totalFeeds,
			"processed_feeds", processedFeeds,
			"remaining_feeds", totalFeeds-processedFeeds)
	}

	logger.Logger.Info("Got source URLs", "count", len(urls), "has_cursor", lastCreatedAt != nil)
	return urls, finalCreatedAt, finalID, nil
}

// GetFeedStatistics returns statistics about feeds processing
func GetFeedStatistics(ctx context.Context, db *pgxpool.Pool) (totalFeeds int, processedFeeds int, err error) {
	// Get total non-MP3 feeds count
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM feeds WHERE link NOT LIKE '%.mp3'").Scan(&totalFeeds)
	if err != nil {
		logger.Logger.Error("Failed to get total non-MP3 feeds count", "error", err)
		return 0, 0, err
	}

	// Get processed non-MP3 feeds count (feeds that have corresponding articles)
	err = db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT f.link)
		FROM feeds f
		INNER JOIN articles a ON f.link = a.url
		WHERE f.link NOT LIKE '%.mp3'
	`).Scan(&processedFeeds)
	if err != nil {
		logger.Logger.Error("Failed to get processed non-MP3 feeds count", "error", err)
		return 0, 0, err
	}

	logger.Logger.Info("Feed statistics (non-MP3 only)", "total_feeds", totalFeeds, "processed_feeds", processedFeeds, "remaining_feeds", totalFeeds-processedFeeds)
	return totalFeeds, processedFeeds, nil
}
