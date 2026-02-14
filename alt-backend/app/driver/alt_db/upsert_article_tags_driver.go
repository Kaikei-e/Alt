package alt_db

import (
	"context"
	"fmt"
)

// UpsertArticleTags upserts tags for an article.
// It first upserts into feed_tags (by feed_id + tag_name), then links via article_tags.
func (r *AltDBRepository) UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []TagUpsertItem) (int32, error) {
	if len(tags) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var upserted int32
	for _, tag := range tags {
		// Upsert into feed_tags
		var feedTagID string
		err := tx.QueryRow(ctx, `
			INSERT INTO feed_tags (feed_id, tag_name, confidence)
			VALUES ($1::uuid, $2, $3)
			ON CONFLICT (feed_id, tag_name) DO UPDATE SET
				confidence = EXCLUDED.confidence,
				updated_at = CURRENT_TIMESTAMP
			RETURNING id
		`, feedID, tag.Name, tag.Confidence).Scan(&feedTagID)
		if err != nil {
			return 0, fmt.Errorf("upsert feed_tag %q: %w", tag.Name, err)
		}

		// Link article to tag via article_tags
		_, err = tx.Exec(ctx, `
			INSERT INTO article_tags (article_id, feed_tag_id)
			VALUES ($1::uuid, $2::uuid)
			ON CONFLICT (article_id, feed_tag_id) DO NOTHING
		`, articleID, feedTagID)
		if err != nil {
			return 0, fmt.Errorf("insert article_tag: %w", err)
		}

		upserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return upserted, nil
}

// BatchUpsertArticleTags upserts tags for multiple articles in a single transaction.
func (r *AltDBRepository) BatchUpsertArticleTags(ctx context.Context, items []BatchUpsertTagItem) (int32, error) {
	if len(items) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var totalUpserted int32
	for _, item := range items {
		for _, tag := range item.Tags {
			var feedTagID string
			err := tx.QueryRow(ctx, `
				INSERT INTO feed_tags (feed_id, tag_name, confidence)
				VALUES ($1::uuid, $2, $3)
				ON CONFLICT (feed_id, tag_name) DO UPDATE SET
					confidence = EXCLUDED.confidence,
					updated_at = CURRENT_TIMESTAMP
				RETURNING id
			`, item.FeedID, tag.Name, tag.Confidence).Scan(&feedTagID)
			if err != nil {
				return 0, fmt.Errorf("upsert feed_tag %q for article %s: %w", tag.Name, item.ArticleID, err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO article_tags (article_id, feed_tag_id)
				VALUES ($1::uuid, $2::uuid)
				ON CONFLICT (article_id, feed_tag_id) DO NOTHING
			`, item.ArticleID, feedTagID)
			if err != nil {
				return 0, fmt.Errorf("insert article_tag for article %s: %w", item.ArticleID, err)
			}

			totalUpserted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return totalUpserted, nil
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
