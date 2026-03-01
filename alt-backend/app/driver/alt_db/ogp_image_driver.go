package alt_db

import (
	"alt/utils/logger"
	"context"
	"fmt"
	"log/slog"
)

// FetchUnwarmedOgImageURLs returns OGP image URLs from recent feeds that are
// not yet cached in image_proxy_cache. SHA256 hashing is done in SQL to match
// the url_hash column used by the image proxy cache.
func (r *AltDBRepository) FetchUnwarmedOgImageURLs(ctx context.Context, limit int) ([]string, error) {
	const query = `
		SELECT DISTINCT f.og_image_url
		FROM feeds f
		WHERE f.og_image_url IS NOT NULL
		  AND f.og_image_url != ''
		  AND f.updated_at >= NOW() - INTERVAL '2 hours'
		  AND NOT EXISTS (
		      SELECT 1 FROM image_proxy_cache ipc
		      WHERE ipc.url_hash = encode(sha256(f.og_image_url::bytea), 'hex')
		        AND ipc.expires_at > NOW()
		  )
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching unwarmed OGP image URLs", "error", err)
		return nil, fmt.Errorf("fetch unwarmed og image URLs: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scan og image URL: %w", err)
		}
		urls = append(urls, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate og image URLs: %w", err)
	}

	slog.InfoContext(ctx, "Fetched unwarmed OGP image URLs", "count", len(urls))
	return urls, nil
}
