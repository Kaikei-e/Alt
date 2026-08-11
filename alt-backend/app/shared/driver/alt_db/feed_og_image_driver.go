package alt_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"alt/domain"
)

// FetchFeedOgImageTargets answers, for feeds a reader has brought into view,
// what the resolver needs in order to decide whether to touch the origin.
//
// The already-held image is COALESCEd across both places one can live: a
// previous on-demand resolution, then the RSS column. A hit in either means
// there is nothing to fetch, and preferring the resolved value keeps a scraped
// result from being shadowed by a stale RSS pointer.
//
// suppressed is computed here rather than returned raw because the rule is a
// property of the row, not of the caller: a refusal stands until its retry_after
// passes, and a NULL retry_after means it stands for the rest of the window.
func (r *FeedRepository) FetchFeedOgImageTargets(ctx context.Context, feedIDs []string) ([]domain.FeedOgImageTarget, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if len(feedIDs) == 0 {
		// Grids ask on every viewport change and most reveal nothing new.
		return nil, nil
	}

	const query = `
		SELECT
		    f.id::text                                         AS feed_id,
		    COALESCE(f.website_url, '')                        AS page_url,
		    COALESCE(
		        CASE WHEN foi.state = 'resolved' THEN foi.og_image_url END,
		        f.og_image_url,
		        ''
		    )                                                  AS og_image_url,
		    COALESCE(
		        foi.state = 'unavailable'
		        AND (foi.retry_after IS NULL OR foi.retry_after > NOW()),
		        false
		    )                                                  AS suppressed
		FROM feeds f
		LEFT JOIN feed_og_images foi ON foi.feed_id = f.id
		WHERE f.id = ANY($1::uuid[])
	`

	rows, err := r.pool.Query(ctx, query, feedIDs)
	if err != nil {
		return nil, fmt.Errorf("query feed og image targets (%d ids): %w", len(feedIDs), err)
	}
	defer rows.Close()

	targets := make([]domain.FeedOgImageTarget, 0, len(feedIDs))
	for rows.Next() {
		var t domain.FeedOgImageTarget
		if err := rows.Scan(&t.FeedID, &t.PageURL, &t.OgImageURL, &t.Suppressed); err != nil {
			return nil, fmt.Errorf("scan feed og image target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// SaveFeedOgImage records one resolution outcome, success or refusal.
//
// An empty ogImageURL writes state='unavailable' with a NULL url, which the
// table's CHECK constraint enforces: a refusal that carried a URL, or a
// resolution that carried none, would read to the query above as the opposite
// of what happened.
//
// retryAfter of zero is stored as NULL rather than as the current instant.
// NULL means "not within this retention window" — the settled answer a
// robots.txt disallow or a page with no og:image tag deserves. Writing "now"
// instead would make the next scroll past that card re-request the origin.
func (r *FeedRepository) SaveFeedOgImage(
	ctx context.Context,
	feedID, ogImageURL string,
	refusal domain.OgImageRefusal,
	retryAfter time.Duration,
) error {
	if r == nil || r.pool == nil {
		return errors.New("database connection not available")
	}

	state := "resolved"
	var url any = ogImageURL
	if ogImageURL == "" {
		state = "unavailable"
		url = nil
	}

	var retryAt any
	if retryAfter > 0 {
		retryAt = time.Now().Add(retryAfter)
	}

	const query = `
		INSERT INTO feed_og_images (feed_id, state, og_image_url, reason, retry_after)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)
		ON CONFLICT (feed_id) DO UPDATE SET
		    state        = EXCLUDED.state,
		    og_image_url = EXCLUDED.og_image_url,
		    reason       = EXCLUDED.reason,
		    retry_after  = EXCLUDED.retry_after,
		    attempts     = feed_og_images.attempts + 1,
		    created_at   = clock_timestamp()
	`

	if _, err := r.pool.Exec(ctx, query, feedID, state, url, string(refusal), retryAt); err != nil {
		return fmt.Errorf("save feed og image %s: %w", feedID, err)
	}
	return nil
}

// CleanupExpiredFeedOgImages enforces the copyright retention window on
// on-demand resolutions, and returns how many rows went.
func (r *FeedRepository) CleanupExpiredFeedOgImages(ctx context.Context, ttl time.Duration) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}

	const query = `DELETE FROM feed_og_images WHERE created_at < $1`

	result, err := r.pool.Exec(ctx, query, time.Now().Add(-ttl))
	if err != nil {
		return 0, fmt.Errorf("cleanup expired feed og images: %w", err)
	}
	return result.RowsAffected(), nil
}
