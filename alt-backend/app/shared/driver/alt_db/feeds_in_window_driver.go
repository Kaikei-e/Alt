package alt_db

import (
	"alt/domain"
	"alt/utils/constants"
	"alt/utils/logger"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const feedsInWindowQuery = `
SELECT
    COUNT(*) OVER() AS total_count,
    f.id,
    f.title,
    f.description,
    f.website_url,
    f.pub_date,
    f.created_at,
    f.updated_at,
    (SELECT a.id::text FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1) AS article_id,
    f.feed_link_id,
    f.og_image_url
FROM feeds f
WHERE f.created_at >= $1 AND f.created_at < $2
ORDER BY f.created_at DESC, f.id DESC
OFFSET $3
LIMIT $4`

// FetchFeedsInWindow retrieves RSS feeds created within a time window with deterministic ordering.
func (r *FeedRepository) FetchFeedsInWindow(ctx context.Context, query domain.FeedsInWindowQuery) (*domain.FeedsInWindowPage, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, errors.New("page and page_size must be positive")
	}
	// Prevent DoS attack via excessive memory allocation (CWE-770)
	if query.PageSize > constants.MaxRecapPageSize {
		return nil, fmt.Errorf("page_size exceeds maximum allowed value of %d", constants.MaxRecapPageSize)
	}

	offset := (query.Page - 1) * query.PageSize

	rows, err := r.pool.Query(ctx, feedsInWindowQuery, query.From, query.To, offset, query.PageSize)
	if err != nil {
		logger.SafeErrorContext(ctx, "feeds in window query failed", "error", err, "from", query.From, "to", query.To)
		return nil, fmt.Errorf("fetch feeds in window: %w", err)
	}
	defer rows.Close()

	feeds := make([]domain.FeedRow, 0, query.PageSize)
	totalCount := 0

	for rows.Next() {
		var (
			rowTotal    int
			feedID      string
			title       string
			description string
			websiteURL  string
			pubDate     time.Time
			createdAt   time.Time
			updatedAt   time.Time
			articleID   sql.NullString
			feedLinkID  sql.NullString
			ogImageURL  sql.NullString
		)

		if err := rows.Scan(
			&rowTotal,
			&feedID,
			&title,
			&description,
			&websiteURL,
			&pubDate,
			&createdAt,
			&updatedAt,
			&articleID,
			&feedLinkID,
			&ogImageURL,
		); err != nil {
			logger.SafeErrorContext(ctx, "feeds in window scan failed", "error", err)
			return nil, fmt.Errorf("scan feeds in window: %w", err)
		}

		totalCount = rowTotal

		feed := domain.FeedRow{
			ID:          feedID,
			Title:       title,
			Description: description,
			WebsiteURL:  websiteURL,
			PubDate:     pubDate,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			ArticleID:   nullStringPtr(articleID),
			IsRead:      false,
			FeedLinkID:  nullStringPtr(feedLinkID),
			OgImageURL:  nullStringPtr(ogImageURL),
		}

		feeds = append(feeds, feed)
	}

	if err := rows.Err(); err != nil {
		logger.SafeErrorContext(ctx, "iteration over feeds in window failed", "error", err)
		return nil, fmt.Errorf("iterate feeds in window: %w", err)
	}

	hasMore := totalCount > 0 && offset+len(feeds) < totalCount

	result := &domain.FeedsInWindowPage{
		Total:    totalCount,
		Page:     query.Page,
		PageSize: query.PageSize,
		HasMore:  hasMore,
		Feeds:    feeds,
	}

	logger.SafeInfoContext(ctx, "fetched feeds in window",
		"count", len(feeds),
		"total", totalCount,
		"page", query.Page,
		"page_size", query.PageSize,
	)

	return result, nil
}
