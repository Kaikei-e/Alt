package alt_db

import (
	"alt/domain"
	"context"
	"time"
)

func (a *AltDBRepository) SearchByTitle(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	queryString := `
		SELECT title, link, description, pub_date, created_at FROM feeds
		WHERE title ILIKE $1
		ORDER BY created_at DESC
		LIMIT 20
	`
	rows, err := a.pool.Query(ctx, queryString, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	feeds := make([]*domain.FeedItem, 0)
	for rows.Next() {
		var feed domain.FeedItem
		var pubDate *time.Time // pub_date can be null, so use pointer
		var createdAt time.Time
		err := rows.Scan(&feed.Title, &feed.Link, &feed.Description, &pubDate, &createdAt)
		if err != nil {
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
	return feeds, nil
}
