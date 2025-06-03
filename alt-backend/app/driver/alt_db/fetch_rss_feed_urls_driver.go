package alt_db

import (
	"alt/utils/logger"
	"context"
	"net/url"
)

func (r *AltDBRepository) FetchRSSFeedURLs(ctx context.Context) ([]url.URL, error) {
	rows, err := r.db.Query(ctx, "SELECT url FROM feed_links")
	if err != nil {
		logger.Logger.Error("Error fetching RSS links", "error", err)
		return nil, err
	}
	defer rows.Close()

	links := []url.URL{}
	for rows.Next() {
		var link string
		err := rows.Scan(&link)
		if err != nil {
			logger.Logger.Error("Error scanning RSS link", "error", err)
			return nil, err
		}

		linkURL, err := url.Parse(link)
		if err != nil {
			logger.Logger.Error("Error parsing RSS link", "error", err)
			return nil, err
		}

		logger.Logger.Info("Found RSS link", "link", linkURL.String())
		links = append(links, *linkURL)
	}

	return links, nil
}
