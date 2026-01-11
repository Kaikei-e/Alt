package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"
)

func (r *AltDBRepository) FetchRSSFeedURLs(ctx context.Context) ([]url.URL, error) {
	// LEFT JOIN ensures feeds without availability records (new feeds) are still returned
	query := `
		SELECT fl.url FROM feed_links fl
		LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id
		WHERE fla.is_active IS NULL OR fla.is_active = true`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		logger.SafeError("Error fetching RSS links", "error", err)
		return nil, errors.New("error fetching RSS links")
	}
	defer rows.Close()

	links := []url.URL{}
	for rows.Next() {
		var link string
		err := rows.Scan(&link)
		if err != nil {
			logger.SafeError("Error scanning RSS link", "error", err)
			return nil, errors.New("error scanning RSS link")
		}

		linkURL, err := url.Parse(link)
		if err != nil {
			logger.SafeError("Error parsing RSS link - invalid URL format", "url", link, "error", err)
			continue // Skip invalid URLs instead of failing entirely
		}

		// Log detailed information about each URL for debugging
		logger.SafeInfo("Found RSS link in database",
			"url", linkURL.String(),
			"scheme", linkURL.Scheme,
			"host", linkURL.Host,
			"path", linkURL.Path)

		links = append(links, *linkURL)
	}

	logger.SafeInfo("RSS feed URL fetch summary", "total_found", len(links))
	return links, nil
}
