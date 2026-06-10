package sovereign_db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TrailFootprint is the domain representation of one footprint on the trail
// spine. verb / item_key / occurred_at are projected from the event log;
// title / excerpt / tags are enriched at read time from knowledge_home_items.
type TrailFootprint struct {
	UserID          uuid.UUID
	TenantID        uuid.UUID
	FootprintKey    string
	Verb            string
	ItemKey         string
	Title           string
	Excerpt         string
	Tags            []string
	Note            string
	SourceEventType string
	OccurredAt      time.Time
}

// UpsertTrailFootprint writes one footprint idempotently. Re-projection of the
// same source event reproduces the same row (merge-safe on footprint_key).
func (r *Repository) UpsertTrailFootprint(ctx context.Context, fp TrailFootprint, projectionVersion int) error {
	const q = `
INSERT INTO knowledge_trail_footprints
  (user_id, tenant_id, footprint_key, verb, item_key, note, source_event_type, occurred_at, projection_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, footprint_key) DO UPDATE SET
  verb = EXCLUDED.verb,
  item_key = EXCLUDED.item_key,
  note = EXCLUDED.note,
  source_event_type = EXCLUDED.source_event_type,
  occurred_at = EXCLUDED.occurred_at,
  projection_version = EXCLUDED.projection_version`
	var note *string
	if fp.Note != "" {
		note = &fp.Note
	}
	if _, err := r.pool.Exec(ctx, q,
		fp.UserID, fp.TenantID, fp.FootprintKey, fp.Verb, fp.ItemKey,
		note, fp.SourceEventType, fp.OccurredAt, projectionVersion,
	); err != nil {
		return fmt.Errorf("UpsertTrailFootprint: %w", err)
	}
	return nil
}

// GetTrailFootprints returns the user's footprint spine in reverse-chronological
// order. Display fields are LEFT JOINed from knowledge_home_items by item_key —
// a read-time enrichment, never a projection-time cross-model read.
func (r *Repository) GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]TrailFootprint, string, bool, error) {
	fetchLimit := limit + 1
	args := []interface{}{userID}
	var where strings.Builder
	where.WriteString(`WHERE f.user_id = $1`)
	argPos := 2
	if cursor != "" {
		occurredAt, footprintKey, err := decodeTrailCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetTrailFootprints: invalid cursor: %w", err)
		}
		where.WriteString(fmt.Sprintf(` AND (f.occurred_at, f.footprint_key) < ($%d, $%d)`, argPos, argPos+1))
		args = append(args, occurredAt, footprintKey)
		argPos += 2
	}

	query := fmt.Sprintf(`
SELECT f.user_id, f.tenant_id, f.footprint_key, f.verb, f.item_key,
       COALESCE(f.note, ''), f.source_event_type, f.occurred_at,
       COALESCE(khi.title, ''), COALESCE(khi.summary_excerpt, ''), COALESCE(khi.tags_json, '[]')
FROM knowledge_trail_footprints f
LEFT JOIN knowledge_home_items khi
  ON khi.user_id = f.user_id
  AND khi.item_key = f.item_key
  AND khi.projection_version = COALESCE((
    SELECT version FROM knowledge_projection_versions
    WHERE status = 'active' ORDER BY version DESC LIMIT 1
  ), 1)
%s
ORDER BY f.occurred_at DESC, f.footprint_key DESC
LIMIT $%d`, where.String(), argPos)
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetTrailFootprints: %w", err)
	}
	defer rows.Close()

	var footprints []TrailFootprint
	for rows.Next() {
		var fp TrailFootprint
		var tagsJSON []byte
		if err := rows.Scan(
			&fp.UserID, &fp.TenantID, &fp.FootprintKey, &fp.Verb, &fp.ItemKey,
			&fp.Note, &fp.SourceEventType, &fp.OccurredAt,
			&fp.Title, &fp.Excerpt, &tagsJSON,
		); err != nil {
			return nil, "", false, fmt.Errorf("GetTrailFootprints scan: %w", err)
		}
		_ = json.Unmarshal(tagsJSON, &fp.Tags)
		footprints = append(footprints, fp)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("GetTrailFootprints rows: %w", err)
	}

	hasMore := len(footprints) > limit
	if hasMore {
		footprints = footprints[:limit]
	}
	var nextCursor string
	if hasMore && len(footprints) > 0 {
		last := footprints[len(footprints)-1]
		nextCursor = encodeTrailCursor(last.OccurredAt, last.FootprintKey)
	}
	return footprints, nextCursor, hasMore, nil
}

func encodeTrailCursor(occurredAt time.Time, footprintKey string) string {
	raw := occurredAt.UTC().Format(time.RFC3339Nano) + "|" + footprintKey
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeTrailCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("decode cursor: %w", err)
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse cursor time: %w", err)
	}
	return occurredAt, parts[1], nil
}
