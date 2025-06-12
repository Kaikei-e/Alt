package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"
)

func (r *AltDBRepository) FetchRSSFeedURLs(ctx context.Context) ([]url.URL, error) {
	rows, err := r.pool.Query(ctx, "SELECT url FROM feed_links")
	if err != nil {
		logger.Logger.Error("Error fetching RSS links", "error", err)
		return nil, errors.New("error fetching RSS links")
	}
	defer rows.Close()

	links := []url.URL{}
	for rows.Next() {
		var link string
		err := rows.Scan(&link)
		if err != nil {
			logger.Logger.Error("Error scanning RSS link", "error", err)
			return nil, errors.New("error scanning RSS link")
		}

		linkURL, err := url.Parse(link)
		if err != nil {
			logger.Logger.Error("Error parsing RSS link - invalid URL format", "url", link, "error", err)
			continue // Skip invalid URLs instead of failing entirely
		}

		// Log detailed information about each URL for debugging
		logger.Logger.Info("Found RSS link in database",
			"url", linkURL.String(),
			"scheme", linkURL.Scheme,
			"host", linkURL.Host,
			"path", linkURL.Path)

		links = append(links, *linkURL)
	}

	logger.Logger.Info("RSS feed URL fetch summary", "total_found", len(links))
	return links, nil
}
