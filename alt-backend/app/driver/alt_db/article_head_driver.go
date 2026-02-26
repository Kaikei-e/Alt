package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SaveArticleHead stores or updates the <head> section and og:image URL for an article.
func (r *AltDBRepository) SaveArticleHead(ctx context.Context, articleID, headHTML, ogImageURL string) error {
	if r == nil || r.pool == nil {
		return errors.New("database connection not available")
	}

	query := `
		INSERT INTO article_heads (article_id, head_html, og_image_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (article_id) DO UPDATE
		SET head_html = EXCLUDED.head_html,
		    og_image_url = EXCLUDED.og_image_url,
		    created_at = CURRENT_TIMESTAMP
	`

	_, err := r.pool.Exec(ctx, query, articleID, headHTML, ogImageURL)
	if err != nil {
		return fmt.Errorf("failed to save article head: %w", err)
	}

	return nil
}

// FetchArticleHeadByArticleID retrieves the article head metadata for a given article ID.
// Returns nil, nil if not found.
func (r *AltDBRepository) FetchArticleHeadByArticleID(ctx context.Context, articleID string) (*domain.ArticleHead, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	query := `
		SELECT id, article_id, head_html, COALESCE(og_image_url, '') as og_image_url
		FROM article_heads
		WHERE article_id = $1
	`

	var head domain.ArticleHead
	err := r.pool.QueryRow(ctx, query, articleID).Scan(
		&head.ID,
		&head.ArticleID,
		&head.HeadHTML,
		&head.OgImageURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch article head: %w", err)
	}

	return &head, nil
}

// FetchOgImageURLByArticleID retrieves only the og_image_url for a given article ID.
// Returns empty string if not found.
func (r *AltDBRepository) FetchOgImageURLByArticleID(ctx context.Context, articleID string) (string, error) {
	if r == nil || r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `SELECT COALESCE(og_image_url, '') FROM article_heads WHERE article_id = $1`
	var ogImageURL string
	err := r.pool.QueryRow(ctx, query, articleID).Scan(&ogImageURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to fetch og_image_url: %w", err)
	}

	return ogImageURL, nil
}

// FetchOgImageURLsByArticleIDs retrieves og_image_url for multiple article IDs.
// Returns a map of articleID -> ogImageURL (only non-empty entries).
func (r *AltDBRepository) FetchOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if len(articleIDs) == 0 {
		return map[string]string{}, nil
	}

	query := `SELECT article_id, COALESCE(og_image_url, '') FROM article_heads WHERE article_id = ANY($1)`
	rows, err := r.pool.Query(ctx, query, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch og_image_urls: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var articleID, ogURL string
		if err := rows.Scan(&articleID, &ogURL); err != nil {
			continue
		}
		if ogURL != "" {
			result[articleID] = ogURL
		}
	}
	return result, nil
}

// CleanupExpiredArticleHeads deletes article_heads older than the given TTL.
// Returns the number of deleted rows.
func (r *AltDBRepository) CleanupExpiredArticleHeads(ctx context.Context, ttl time.Duration) (int64, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}

	query := `DELETE FROM article_heads WHERE created_at < $1`
	cutoff := time.Now().Add(-ttl)

	result, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired article heads: %w", err)
	}

	return result.RowsAffected(), nil
}
