package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// GetKnowledgeHomeItems returns paginated items for a user, ordered by score DESC, published_at DESC.
// Returns items, nextCursor, hasMore, error.
func (r *AltDBRepository) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]domain.KnowledgeHomeItem, string, bool, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetKnowledgeHomeItems")
	defer span.End()

	var query string
	var args []interface{}

	// Fetch limit+1 to determine hasMore
	fetchLimit := limit + 1

	if cursor == "" {
		query = `SELECT user_id, tenant_id, item_key, item_type, primary_ref_id,
			title, summary_excerpt, tags_json, why_json, score,
			freshness_at, published_at, last_interacted_at, generated_at, updated_at
			FROM knowledge_home_items
			WHERE user_id = $1
			ORDER BY score DESC, published_at DESC
			LIMIT $2`
		args = []interface{}{userID, fetchLimit}
	} else {
		cursorScore, cursorPublishedAt, cursorItemKey, err := decodeCursor(cursor)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "invalid cursor", "error", err)
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: invalid cursor: %w", err)
		}
		query = `SELECT user_id, tenant_id, item_key, item_type, primary_ref_id,
			title, summary_excerpt, tags_json, why_json, score,
			freshness_at, published_at, last_interacted_at, generated_at, updated_at
			FROM knowledge_home_items
			WHERE user_id = $1
			  AND (score, published_at, item_key) < ($2, $3, $4)
			ORDER BY score DESC, published_at DESC
			LIMIT $5`
		args = []interface{}{userID, cursorScore, cursorPublishedAt, cursorItemKey, fetchLimit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
	}
	defer rows.Close()

	var items []domain.KnowledgeHomeItem
	for rows.Next() {
		var item domain.KnowledgeHomeItem
		var tagsJSON, whyJSON []byte
		err := rows.Scan(
			&item.UserID, &item.TenantID, &item.ItemKey, &item.ItemType, &item.PrimaryRefID,
			&item.Title, &item.SummaryExcerpt, &tagsJSON, &whyJSON, &item.Score,
			&item.FreshnessAt, &item.PublishedAt, &item.LastInteractedAt, &item.GeneratedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems scan: %w", err)
		}
		_ = json.Unmarshal(tagsJSON, &item.Tags)
		_ = json.Unmarshal(whyJSON, &item.WhyReasons)
		items = append(items, item)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.Score, last.PublishedAt, last.ItemKey)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(items)))
	return items, nextCursor, hasMore, nil
}

// UpsertKnowledgeHomeItem inserts or updates a knowledge home item.
func (r *AltDBRepository) UpsertKnowledgeHomeItem(ctx context.Context, item domain.KnowledgeHomeItem) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpsertKnowledgeHomeItem")
	defer span.End()

	tagsJSON, _ := json.Marshal(item.Tags)
	whyJSON, _ := json.Marshal(item.WhyReasons)

	query := `INSERT INTO knowledge_home_items
		(user_id, tenant_id, item_key, item_type, primary_ref_id,
		 title, summary_excerpt, tags_json, why_json, score,
		 freshness_at, published_at, last_interacted_at, generated_at, updated_at,
		 projection_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		 title = EXCLUDED.title,
		 summary_excerpt = EXCLUDED.summary_excerpt,
		 tags_json = EXCLUDED.tags_json,
		 why_json = EXCLUDED.why_json,
		 score = EXCLUDED.score,
		 freshness_at = EXCLUDED.freshness_at,
		 published_at = EXCLUDED.published_at,
		 last_interacted_at = EXCLUDED.last_interacted_at,
		 updated_at = EXCLUDED.updated_at,
		 projection_version = EXCLUDED.projection_version`

	_, err := r.pool.Exec(ctx, query,
		item.UserID, item.TenantID, item.ItemKey, item.ItemType, item.PrimaryRefID,
		item.Title, item.SummaryExcerpt, tagsJSON, whyJSON, item.Score,
		item.FreshnessAt, item.PublishedAt, item.LastInteractedAt, item.GeneratedAt, item.UpdatedAt,
		item.ProjectionVersion,
	)
	if err != nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: %w", err)
	}

	return nil
}

// encodeCursor encodes a cursor from score, publishedAt, and itemKey.
func encodeCursor(score float64, publishedAt *time.Time, itemKey string) string {
	var pubStr string
	if publishedAt != nil {
		pubStr = publishedAt.Format(time.RFC3339Nano)
	}
	raw := fmt.Sprintf("%f|%s|%s", score, pubStr, itemKey)
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes a cursor into score, publishedAt, and itemKey.
func decodeCursor(cursor string) (float64, *time.Time, string, error) {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, nil, "", err
	}
	parts := strings.SplitN(string(raw), "|", 3)
	if len(parts) != 3 {
		return 0, nil, "", fmt.Errorf("invalid cursor format")
	}

	var score float64
	_, err = fmt.Sscanf(parts[0], "%f", &score)
	if err != nil {
		return 0, nil, "", fmt.Errorf("invalid cursor score: %w", err)
	}

	var publishedAt *time.Time
	if parts[1] != "" {
		t, err := time.Parse(time.RFC3339Nano, parts[1])
		if err != nil {
			return 0, nil, "", fmt.Errorf("invalid cursor time: %w", err)
		}
		publishedAt = &t
	}

	return score, publishedAt, parts[2], nil
}
