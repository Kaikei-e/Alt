package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

func (r *AltDBRepository) FetchFeedLinks(ctx context.Context) ([]*domain.FeedLink, error) {
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

func (r *AltDBRepository) DeleteFeedLink(ctx context.Context, id uuid.UUID) error {
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
