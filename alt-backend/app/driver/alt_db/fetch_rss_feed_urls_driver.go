package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

func (r *AltDBRepository) FetchRSSFeedURLs(ctx context.Context) ([]domain.FeedLink, error) {
	// LEFT JOIN ensures feeds without availability records (new feeds) are still returned
	query := `
		SELECT fl.id, fl.url FROM feed_links fl
		LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id
		WHERE fla.is_active IS NULL OR fla.is_active = true`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching RSS links", "error", err)
		return nil, errors.New("error fetching RSS links")
	}
	defer rows.Close()

	var feedLinks []domain.FeedLink
	for rows.Next() {
		var id uuid.UUID
		var link string
		err := rows.Scan(&id, &link)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error scanning RSS link", "error", err)
			return nil, errors.New("error scanning RSS link")
		}

		logger.SafeInfoContext(ctx, "Found RSS link in database",
			"id", id.String(),
			"url", link)

		feedLinks = append(feedLinks, domain.FeedLink{ID: id, URL: link})
	}

	logger.SafeInfoContext(ctx, "RSS feed URL fetch summary", "total_found", len(feedLinks))
	return feedLinks, nil
}
