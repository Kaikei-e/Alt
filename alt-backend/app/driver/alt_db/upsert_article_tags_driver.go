package alt_db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// upsertTagCTE combines feed_tags upsert and article_tags link into a single query.
// This eliminates the N+1 pattern of separate QueryRow + Exec per tag.
const upsertTagCTE = `
	WITH ft AS (
		INSERT INTO feed_tags (feed_id, tag_name, confidence)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (feed_id, tag_name) DO UPDATE SET
			confidence = EXCLUDED.confidence,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	)
	INSERT INTO article_tags (article_id, feed_tag_id)
	SELECT $4::uuid, ft.id FROM ft
	ON CONFLICT (article_id, feed_tag_id) DO NOTHING
`

// UpsertArticleTags upserts tags for an article.
// It first upserts into feed_tags (by feed_id + tag_name), then links via article_tags.
// Uses pgx.Batch to send all tag operations in a single round trip.
func (r *AltDBRepository) UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []TagUpsertItem) (int32, error) {
	if len(tags) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, tag := range tags {
		batch.Queue(upsertTagCTE, feedID, tag.Name, tag.Confidence, articleID)
	}

	br := tx.SendBatch(ctx, batch)
	for _, tag := range tags {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return 0, fmt.Errorf("upsert tag %q: %w", tag.Name, err)
		}
	}
	if err := br.Close(); err != nil {
		return 0, fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return int32(len(tags)), nil
}

// BatchUpsertArticleTags upserts tags for multiple articles in a single transaction.
// Uses pgx.Batch to send all tag operations in a single round trip.
func (r *AltDBRepository) BatchUpsertArticleTags(ctx context.Context, items []BatchUpsertTagItem) (int32, error) {
	if len(items) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	totalTags := 0
	for _, item := range items {
		for _, tag := range item.Tags {
			batch.Queue(upsertTagCTE, item.FeedID, tag.Name, tag.Confidence, item.ArticleID)
			totalTags++
		}
	}

	br := tx.SendBatch(ctx, batch)
	for i := 0; i < totalTags; i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return 0, fmt.Errorf("batch upsert tag %d: %w", i, err)
		}
	}
	if err := br.Close(); err != nil {
		return 0, fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return int32(totalTags), nil
}

// TagUpsertItem represents a tag to upsert.
type TagUpsertItem struct {
	Name       string
	Confidence float32
}

// BatchUpsertTagItem holds data for a single article's tag upsert in a batch.
type BatchUpsertTagItem struct {
	ArticleID string
	FeedID    string
	Tags      []TagUpsertItem
}
