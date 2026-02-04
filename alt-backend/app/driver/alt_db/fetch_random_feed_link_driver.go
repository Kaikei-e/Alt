package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// FetchRandomFeedLink retrieves a random feed link from the user's subscriptions.
// Uses PostgreSQL's RANDOM() function to select a random row.
// Deprecated: Use FetchRandomFeed instead which returns more metadata.
func (r *AltDBRepository) FetchRandomFeedLink(ctx context.Context) (*domain.FeedLink, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT id, url
		FROM feed_links
		ORDER BY RANDOM()
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query)

	var id uuid.UUID
	var url string
	err := row.Scan(&id, &url)
	if err != nil {
		if err.Error() == "no rows in result set" {
			logger.Logger.InfoContext(ctx, "no feed links found")
			return nil, nil
		}
		logger.SafeErrorContext(ctx, "error fetching random feed link", "error", err)
		return nil, errors.New("error fetching random feed link")
	}

	logger.SafeInfoContext(ctx, "fetched random feed link", "id", id.String())
	return &domain.FeedLink{ID: id, URL: url}, nil
}

// FetchRandomFeed retrieves a random feed from the feeds table.
// Returns the feed with title, description, and link for Tag Trail feature.
// Uses PostgreSQL's RANDOM() function to select a random row.
func (r *AltDBRepository) FetchRandomFeed(ctx context.Context) (*domain.Feed, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT id, title, description, link
		FROM feeds
		ORDER BY RANDOM()
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query)

	var id uuid.UUID
	var title string
	var description sql.NullString
	var link string

	err := row.Scan(&id, &title, &description, &link)
	if err != nil {
		if err.Error() == "no rows in result set" {
			logger.Logger.InfoContext(ctx, "no feeds found")
			return nil, nil
		}
		logger.SafeErrorContext(ctx, "error fetching random feed", "error", err)
		return nil, errors.New("error fetching random feed")
	}

	logger.SafeInfoContext(ctx, "fetched random feed", "id", id.String(), "title", title)

	return &domain.Feed{
		ID:          id,
		Title:       title,
		Description: description.String, // converts NULL to empty string
		Link:        link,
	}, nil
}
