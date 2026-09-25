package sovereign_db

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EngagedDwellMs is the raw-dwell threshold at or above which a walked branch
// counts as an engaged walk in the path-wear derivation. It inherits the
// ADR-000908 30s rule, but as a read-time derivation constant (D18): the
// emitted event carries only the raw dwell, so changing this needs no
// reproject — every read re-derives wear from the raw measurements. Exported
// so the projector's branch-KPI logging (Wave 10) references the same
// constant rather than duplicating the literal.
const EngagedDwellMs = int64(30_000)

// EngagementVerbs are the footprint verbs a branch's why can truthfully name —
// "Because you read / listened to / asked about this" (core-concept §C4,
// anchored why). They are the only verbs that may anchor a branch or count as
// contact with a thread. `dismissed` is deliberately absent: it is a refusal,
// so it neither backs a why nor evidences interest in picking a thread back
// up. Exported so the planner can pin its why phrasing to the same set.
var EngagementVerbs = []string{"read", "asked", "listened"}

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
	Wear            string
	// ContactCount is how many acts of this verb on this item are collapsed
	// into this row (>= 1); OccurredAt is the latest contact and
	// FirstOccurredAt the earliest (D24 — repeated reads never add rows).
	ContactCount    int
	FirstOccurredAt time.Time
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

func buildTrailFootprintsFilter(cursor string, filterTags []string, baseArgPos int) (string, []any, int, error) {
	var where strings.Builder
	where.WriteString(`WHERE TRUE`)
	argPos := baseArgPos
	var args []any
	if cursor != "" {
		occurredAt, footprintKey, err := decodeTrailCursor(cursor)
		if err != nil {
			return "", nil, baseArgPos, fmt.Errorf("invalid cursor: %w", err)
		}
		fmt.Fprintf(&where, ` AND (f.occurred_at, f.footprint_key) < ($%d, $%d)`, argPos, argPos+1)
		args = append(args, occurredAt, footprintKey)
		argPos += 2
	}
	if len(filterTags) > 0 {
		fmt.Fprintf(&where, ` AND EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(COALESCE(khi.tags_json, '[]')) AS tag_name
			WHERE tag_name = ANY($%d)
		)`, argPos)
		args = append(args, filterTags)
		argPos++
	}
	return where.String(), args, argPos, nil
}

func scanTrailFootprintRow(scanner rowScanner, userID uuid.UUID) (TrailFootprint, error) {
	fp := TrailFootprint{UserID: userID}
	var tagsJSON []byte
	if err := scanner.Scan(
		&fp.TenantID, &fp.FootprintKey, &fp.Verb, &fp.ItemKey,
		&fp.Note, &fp.SourceEventType, &fp.OccurredAt,
		&fp.FirstOccurredAt, &fp.ContactCount,
		&fp.Title, &fp.Excerpt, &tagsJSON, &fp.Wear,
	); err != nil {
		return TrailFootprint{}, err
	}
	unmarshalJSONWarn(tagsJSON, &fp.Tags, "tags_json")
	return fp, nil
}

// GetTrailFootprints returns the user's footprint spine in reverse-chronological
// order. Display fields are LEFT JOINed from knowledge_home_items by item_key —
// a read-time enrichment, never a projection-time cross-model read. Path wear is
// derived per item over ALL the user's footprints (CTE), so it is stable across
// pages. filterTags applies the theme lens (item must carry one of the tags).
func (r *Repository) GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) ([]TrailFootprint, string, bool, error) {
	fetchLimit := limit + 1
	whereClause, filterArgs, argPos, err := buildTrailFootprintsFilter(cursor, filterTags, 3)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetTrailFootprints: %w", err)
	}
	args := make([]any, 0, 2+len(filterArgs)+1)
	args = append(args, userID, EngagedDwellMs)
	args = append(args, filterArgs...)

	// item_wear aggregates over the whole spine so the wear band does not change
	// as the user pages. has_ask or a deep revisit count reads as "deep".
	// item_engagement folds the act-outcome side table: a raw dwell at or above
	// the engaged threshold ($2, a Go constant — D20) or a Loop-era engaged
	// label marks the item as substantively walked. An engaged walk lifts the
	// band at least to worn; engaged plus a revisit reads as deep.
	// collapsed folds repeated contacts with one (item, verb) into a single
	// spine row (D24): the row sorts by its latest contact, remembers its
	// first, and carries the contact count. Wear still folds over raw rows —
	// a revisit no longer adds a row, but it still deepens the path.
	query := fmt.Sprintf(`
WITH item_wear AS (
  SELECT item_key, count(*) AS cnt, bool_or(verb = 'asked') AS has_ask
  FROM knowledge_trail_footprints
  WHERE user_id = $1
  GROUP BY item_key
),
item_engagement AS (
  SELECT item_key, TRUE AS engaged
  FROM knowledge_trail_act_outcomes
  WHERE user_id = $1
    AND ((dwell_ms IS NOT NULL AND dwell_ms >= $2)
         OR legacy_outcome IN ('engaged', 'deep_engagement'))
  GROUP BY item_key
),
collapsed AS (
  SELECT tenant_id, item_key, verb,
         count(*) AS contact_count,
         min(occurred_at) AS first_occurred_at,
         max(occurred_at) AS occurred_at,
         (array_agg(footprint_key ORDER BY occurred_at DESC, footprint_key DESC))[1] AS footprint_key,
         (array_agg(note ORDER BY occurred_at DESC, footprint_key DESC))[1] AS note,
         (array_agg(source_event_type ORDER BY occurred_at DESC, footprint_key DESC))[1] AS source_event_type
  FROM knowledge_trail_footprints
  WHERE user_id = $1
  GROUP BY tenant_id, item_key, verb
)
SELECT f.tenant_id, f.footprint_key, f.verb, f.item_key,
       COALESCE(f.note, ''), f.source_event_type, f.occurred_at,
       f.first_occurred_at, f.contact_count,
       -- Display title with a readable fallback: a title-less item (upstream
       -- knowledge_home_items.title gap) shows its source host, never the raw
       -- item key. The excerpt is rendered separately, so it is not used here.
       COALESCE(NULLIF(khi.title, ''),
                NULLIF(split_part(split_part(khi.url, '://', 2), '/', 1), ''),
                f.item_key),
       COALESCE(khi.summary_excerpt, ''), COALESCE(khi.tags_json, '[]'),
       CASE WHEN iw.has_ask OR iw.cnt >= 4
                 OR (COALESCE(ie.engaged, FALSE) AND iw.cnt >= 2) THEN 'deep'
            WHEN iw.cnt >= 2 OR COALESCE(ie.engaged, FALSE) THEN 'worn'
            ELSE 'thin' END AS wear
FROM collapsed f
JOIN item_wear iw ON iw.item_key = f.item_key
LEFT JOIN item_engagement ie ON ie.item_key = f.item_key
LEFT JOIN knowledge_home_items khi
  ON khi.user_id = $1
  AND khi.item_key = f.item_key
  AND khi.projection_version = `+activeProjectionVersionSQL+`
%s
ORDER BY f.occurred_at DESC, f.footprint_key DESC
LIMIT $%d`, whereClause, argPos)
	args = append(args, fetchLimit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("GetTrailFootprints: %w", err)
	}
	defer rows.Close()

	var footprints []TrailFootprint
	for rows.Next() {
		fp, err := scanTrailFootprintRow(rows, userID)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetTrailFootprints scan: %w", err)
		}
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
