package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (r *FeedRepository) FetchFeedLinks(ctx context.Context) ([]*domain.FeedLink, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, url FROM feed_links ORDER BY url ASC")
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feed links", "error", err)
		return nil, errors.New("error fetching feed links")
	}
	defer rows.Close()

	links := make([]*domain.FeedLink, 0)
	for rows.Next() {
		var id uuid.UUID
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			logger.SafeErrorContext(ctx, "Error scanning feed link", "error", err)
			return nil, errors.New("error scanning feed links")
		}
		links = append(links, &domain.FeedLink{ID: id, URL: url})
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "Row iteration error", "error", err)
		return nil, errors.New("error iterating feed links")
	}

	return links, nil
}

// FetchFeedLinkIDByURL returns the feed_link ID for a given URL, or nil if not found.
func (r *FeedRepository) FetchFeedLinkIDByURL(ctx context.Context, feedURL string) (*string, error) {
	var id string
	err := r.pool.QueryRow(ctx, "SELECT id FROM feed_links WHERE url = $1", feedURL).Scan(&id)
	if err != nil {
		// Not found is not an error - just return nil
		return nil, nil
	}
	return &id, nil
}

func (r *FeedRepository) FetchFeedLinksWithAvailability(ctx context.Context) ([]*domain.FeedLinkWithHealth, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT fl.id, fl.url, fla.is_active, fla.consecutive_failures, fla.last_failure_at, fla.last_failure_reason FROM feed_links fl LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id ORDER BY fl.url ASC`)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feed links with availability", "error", err)
		return nil, errors.New("error fetching feed links with availability")
	}
	defer rows.Close()

	links := make([]*domain.FeedLinkWithHealth, 0)
	for rows.Next() {
		var id uuid.UUID
		var url string
		var isActive *bool
		var consecutiveFailures *int
		var lastFailureAt *time.Time
		var lastFailureReason *string

		if err := rows.Scan(&id, &url, &isActive, &consecutiveFailures, &lastFailureAt, &lastFailureReason); err != nil {
			logger.SafeErrorContext(ctx, "Error scanning feed link with availability", "error", err)
			return nil, errors.New("error scanning feed links with availability")
		}

		link := &domain.FeedLinkWithHealth{
			FeedLink: domain.FeedLink{ID: id, URL: url},
		}

		if isActive != nil {
			cf := 0
			if consecutiveFailures != nil {
				cf = *consecutiveFailures
			}
			link.Availability = &domain.FeedLinkAvailability{
				FeedLinkID:          id,
				IsActive:            *isActive,
				ConsecutiveFailures: cf,
				LastFailureAt:       lastFailureAt,
				LastFailureReason:   lastFailureReason,
			}
		}

		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "Row iteration error", "error", err)
		return nil, errors.New("error iterating feed links with availability")
	}

	return links, nil
}

func (r *FeedRepository) DeleteFeedLink(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM feed_links WHERE id = $1", id)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error deleting feed link", "error", err, "id", id)
		return errors.New("error deleting feed link")
	}

	if result.RowsAffected() == 0 {
		logger.SafeWarnContext(ctx, "Feed link not found", "id", id)
		return errors.New("feed link not found")
	}

	logger.SafeInfoContext(ctx, "Feed link deleted", "id", id)
	return nil
}
