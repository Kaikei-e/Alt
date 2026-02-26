package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"fmt"
	"time"
)

// GetImageProxyCache retrieves a cached image by URL hash.
// Returns nil if not found or expired.
func (r *AltDBRepository) GetImageProxyCache(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	var entry domain.ImageProxyCacheEntry
	err := r.pool.QueryRow(ctx,
		`SELECT url_hash, original_url, image_data, content_type, width, height, size_bytes, etag, created_at, expires_at
		 FROM image_proxy_cache
		 WHERE url_hash = $1 AND expires_at > $2`,
		urlHash, time.Now(),
	).Scan(
		&entry.URLHash,
		&entry.OriginalURL,
		&entry.Data,
		&entry.ContentType,
		&entry.Width,
		&entry.Height,
		&entry.SizeBytes,
		&entry.ETag,
		&entry.CreatedAt,
		&entry.ExpiresAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		logger.SafeErrorContext(ctx, "Error fetching image proxy cache", "error", err, "urlHash", urlHash)
		return nil, fmt.Errorf("get image proxy cache: %w", err)
	}
	return &entry, nil
}

// SaveImageProxyCache upserts a cached image entry.
func (r *AltDBRepository) SaveImageProxyCache(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO image_proxy_cache (url_hash, original_url, image_data, content_type, width, height, size_bytes, etag, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (url_hash) DO UPDATE SET
		   image_data = EXCLUDED.image_data,
		   content_type = EXCLUDED.content_type,
		   width = EXCLUDED.width,
		   height = EXCLUDED.height,
		   size_bytes = EXCLUDED.size_bytes,
		   etag = EXCLUDED.etag,
		   expires_at = EXCLUDED.expires_at`,
		entry.URLHash,
		entry.OriginalURL,
		entry.Data,
		entry.ContentType,
		entry.Width,
		entry.Height,
		entry.SizeBytes,
		entry.ETag,
		entry.ExpiresAt,
	)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error saving image proxy cache", "error", err, "urlHash", entry.URLHash)
		return fmt.Errorf("save image proxy cache: %w", err)
	}
	return nil
}

// CleanupExpiredImageProxyCache deletes expired cache entries and returns the count.
func (r *AltDBRepository) CleanupExpiredImageProxyCache(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM image_proxy_cache WHERE expires_at <= $1`,
		time.Now(),
	)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error cleaning up expired image proxy cache", "error", err)
		return 0, fmt.Errorf("cleanup expired image proxy cache: %w", err)
	}
	return tag.RowsAffected(), nil
}
