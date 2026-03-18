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
func (r *AltDBRepository) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetKnowledgeHomeItems")
	defer span.End()

	var query strings.Builder
	args := []interface{}{userID}

	// Fetch limit+1 to determine hasMore
	fetchLimit := limit + 1

	query.WriteString(`SELECT user_id, tenant_id, item_key, item_type, primary_ref_id,
		title, summary_excerpt, tags_json, why_json, score,
		freshness_at, published_at, last_interacted_at, generated_at, updated_at,
		summary_state
		FROM knowledge_home_items
		WHERE user_id = $1`)

	argPos := 2
	if filter != nil {
		if len(filter.TagNames) > 0 || len(filter.FeedIDs) > 0 || filter.TimeWindow != "" {
			query.WriteString(` AND item_type = 'article'`)
		}
		if len(filter.TagNames) > 0 {
			query.WriteString(fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(knowledge_home_items.tags_json) AS tag_name
				WHERE tag_name = ANY($%d)
			)`, argPos))
			args = append(args, filter.TagNames)
			argPos++
		}
		if len(filter.FeedIDs) > 0 {
			query.WriteString(fmt.Sprintf(` AND EXISTS (
				SELECT 1 FROM articles a
				WHERE a.id = knowledge_home_items.primary_ref_id
				  AND a.feed_id = ANY($%d)
			)`, argPos))
			args = append(args, filter.FeedIDs)
			argPos++
		}
		if filter.TimeWindow != "" {
			cutoff, err := cutoffFromTimeWindow(filter.TimeWindow)
			if err != nil {
				return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: %w", err)
			}
			query.WriteString(fmt.Sprintf(` AND published_at >= $%d`, argPos))
			args = append(args, cutoff)
			argPos++
		}
	}

	if cursor != "" {
		cursorScore, cursorPublishedAt, cursorItemKey, err := decodeCursor(cursor)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "invalid cursor", "error", err)
			return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: invalid cursor: %w", err)
		}
		query.WriteString(fmt.Sprintf(` AND (score, published_at, item_key) < ($%d, $%d, $%d)`,
			argPos, argPos+1, argPos+2))
		args = append(args, cursorScore, cursorPublishedAt, cursorItemKey)
		argPos += 3
	}

	query.WriteString(fmt.Sprintf(` ORDER BY score DESC, published_at DESC, item_key DESC LIMIT $%d`, argPos))
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query.String(), args...)
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
			&item.SummaryState,
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

func cutoffFromTimeWindow(window string) (time.Time, error) {
	now := time.Now().UTC()
	switch window {
	case "7d":
		return now.Add(-7 * 24 * time.Hour), nil
	case "30d":
		return now.Add(-30 * 24 * time.Hour), nil
	case "90d":
		return now.Add(-90 * 24 * time.Hour), nil
	case "":
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time window: %s", window)
	}
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
		 projection_version, summary_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (user_id, item_key) DO UPDATE SET
		 title = CASE WHEN EXCLUDED.title != '' THEN EXCLUDED.title ELSE knowledge_home_items.title END,
		 summary_excerpt = CASE WHEN EXCLUDED.summary_excerpt != '' THEN EXCLUDED.summary_excerpt ELSE knowledge_home_items.summary_excerpt END,
		 tags_json = CASE WHEN EXCLUDED.tags_json != '[]'::jsonb THEN EXCLUDED.tags_json ELSE knowledge_home_items.tags_json END,
		 why_json = CASE
			 WHEN EXCLUDED.why_json = '[]'::jsonb THEN knowledge_home_items.why_json
			 ELSE (
				 SELECT COALESCE(jsonb_agg(merged.reason ORDER BY merged.code), '[]'::jsonb)
				 FROM (
					 SELECT DISTINCT ON (candidate.code) candidate.code, candidate.reason
					 FROM (
						 SELECT reason->>'code' AS code, reason, 0 AS source_rank
						 FROM jsonb_array_elements(EXCLUDED.why_json) AS reason
						 UNION ALL
						 SELECT reason->>'code' AS code, reason, 1 AS source_rank
						 FROM jsonb_array_elements(COALESCE(knowledge_home_items.why_json, '[]'::jsonb)) AS reason
					 ) AS candidate
					 ORDER BY candidate.code, candidate.source_rank
				 ) AS merged
			 )
		 END,
		 score = GREATEST(EXCLUDED.score, knowledge_home_items.score),
		 freshness_at = COALESCE(EXCLUDED.freshness_at, knowledge_home_items.freshness_at),
		 published_at = COALESCE(EXCLUDED.published_at, knowledge_home_items.published_at),
		 last_interacted_at = COALESCE(EXCLUDED.last_interacted_at, knowledge_home_items.last_interacted_at),
		 updated_at = EXCLUDED.updated_at,
		 projection_version = EXCLUDED.projection_version,
		 summary_state = CASE WHEN EXCLUDED.summary_state = 'ready' THEN 'ready' WHEN EXCLUDED.summary_state != 'missing' THEN EXCLUDED.summary_state ELSE knowledge_home_items.summary_state END`

	_, err := r.pool.Exec(ctx, query,
		item.UserID, item.TenantID, item.ItemKey, item.ItemType, item.PrimaryRefID,
		item.Title, item.SummaryExcerpt, string(tagsJSON), string(whyJSON), item.Score,
		item.FreshnessAt, item.PublishedAt, item.LastInteractedAt, item.GeneratedAt, item.UpdatedAt,
		item.ProjectionVersion, item.SummaryState,
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
