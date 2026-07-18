package sovereign_db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// engagedDwellMs is the raw-dwell threshold at or above which a walked branch
// counts as an engaged walk in the path-wear derivation. It inherits the
// ADR-000908 30s rule, but as a read-time derivation constant (D18): the
// emitted event carries only the raw dwell, so changing this needs no
// reproject — every read re-derives wear from the raw measurements.
const engagedDwellMs = int64(30_000)

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
// a read-time enrichment, never a projection-time cross-model read. Path wear is
// derived per item over ALL the user's footprints (CTE), so it is stable across
// pages. filterTags applies the theme lens (item must carry one of the tags).
func (r *Repository) GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) ([]TrailFootprint, string, bool, error) {
	fetchLimit := limit + 1
	args := []any{userID, engagedDwellMs}
	var where strings.Builder
	where.WriteString(`WHERE f.user_id = $1`)
	argPos := 3
	if cursor != "" {
		occurredAt, footprintKey, err := decodeTrailCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("GetTrailFootprints: invalid cursor: %w", err)
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

	// item_wear aggregates over the whole spine so the wear band does not change
	// as the user pages. has_ask or a deep revisit count reads as "deep".
	// item_engagement folds the act-outcome side table: a raw dwell at or above
	// the engaged threshold ($2, a Go constant — D20) or a Loop-era engaged
	// label marks the item as substantively walked. An engaged walk lifts the
	// band at least to worn; engaged plus a revisit reads as deep.
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
)
SELECT f.user_id, f.tenant_id, f.footprint_key, f.verb, f.item_key,
       COALESCE(f.note, ''), f.source_event_type, f.occurred_at,
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
FROM knowledge_trail_footprints f
JOIN item_wear iw ON iw.item_key = f.item_key
LEFT JOIN item_engagement ie ON ie.item_key = f.item_key
LEFT JOIN knowledge_home_items khi
  ON khi.user_id = f.user_id
  AND khi.item_key = f.item_key
  AND khi.projection_version = `+activeProjectionVersionSQL+`
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
			&fp.Title, &fp.Excerpt, &tagsJSON, &fp.Wear,
		); err != nil {
			return nil, "", false, fmt.Errorf("GetTrailFootprints scan: %w", err)
		}
		unmarshalJSONWarn(tagsJSON, &fp.Tags, "tags_json")
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

// TrailEvidenceRef is one piece of evidence backing a branch.
type TrailEvidenceRef struct {
	RefID string `json:"ref_id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// TrailBranch is the read-model view of a system-proposed branch.
type TrailBranch struct {
	BranchKey     string
	AnchorItemKey string
	RelationKind  string
	Why           string
	EvidenceRefs  []TrailEvidenceRef
	Confidence    string
	TargetItemKey string
	TargetTitle   string
}

// TrailClusterCandidate is a new item that shares tags with the user's followed
// topics and that the user has not yet engaged — the raw material for a Cluster
// branch.
type TrailClusterCandidate struct {
	TargetItemKey string
	TargetTitle   string
	SharedTags    []string
}

// UpsertTrailBranch folds a branch_proposed event into the read model. It never
// downgrades a resolved branch back to open (Wave 5 sets state separately), and
// re-projection of the same event reproduces the same row.
func (r *Repository) UpsertTrailBranch(ctx context.Context, userID, tenantID uuid.UUID, b TrailBranch, createdAt time.Time, projectionVersion int) error {
	refs, err := json.Marshal(b.EvidenceRefs)
	if err != nil {
		return fmt.Errorf("UpsertTrailBranch marshal: %w", err)
	}
	const q = `
INSERT INTO knowledge_trail_branches
  (user_id, tenant_id, branch_key, anchor_item_key, relation_kind, why,
   evidence_refs_json, confidence, target_item_key, target_title, state,
   created_at, projection_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'open', $11, $12)
ON CONFLICT (user_id, branch_key) DO UPDATE SET
  anchor_item_key = EXCLUDED.anchor_item_key,
  relation_kind = EXCLUDED.relation_kind,
  why = EXCLUDED.why,
  evidence_refs_json = EXCLUDED.evidence_refs_json,
  confidence = EXCLUDED.confidence,
  target_item_key = EXCLUDED.target_item_key,
  target_title = EXCLUDED.target_title,
  created_at = EXCLUDED.created_at,
  projection_version = EXCLUDED.projection_version`
	if _, err := r.pool.Exec(ctx, q,
		userID, tenantID, b.BranchKey, b.AnchorItemKey, b.RelationKind, b.Why,
		refs, b.Confidence, b.TargetItemKey, b.TargetTitle, createdAt, projectionVersion,
	); err != nil {
		return fmt.Errorf("UpsertTrailBranch: %w", err)
	}
	return nil
}

// SetTrailBranchState transitions a branch to a resolved state (taken/dismissed),
// folded from trail.branch_resolved.v1. A missing row (orphaned resolution) is a
// no-op rather than a fabricated row; on a full replay the proposed event always
// precedes the resolved one in seq order so the row is present.
func (r *Repository) SetTrailBranchState(ctx context.Context, userID uuid.UUID, branchKey, state string) error {
	const q = `UPDATE knowledge_trail_branches SET state = $3
		WHERE user_id = $1 AND branch_key = $2`
	if _, err := r.pool.Exec(ctx, q, userID, branchKey, state); err != nil {
		return fmt.Errorf("SetTrailBranchState: %w", err)
	}
	return nil
}

// GetOpenTrailBranches returns the user's open branches, newest first.
func (r *Repository) GetOpenTrailBranches(ctx context.Context, userID uuid.UUID) ([]TrailBranch, error) {
	// target_title carries a read-time display fallback for branches whose stored
	// title is empty (title-less targets already in the log, before the planner
	// title gate): live home title → excerpt snippet → source host → item key.
	q := `
SELECT b.branch_key, b.anchor_item_key, b.relation_kind, b.why, b.evidence_refs_json,
       b.confidence, b.target_item_key,
       COALESCE(NULLIF(b.target_title, ''),
                NULLIF(khi.title, ''),
                NULLIF(left(khi.summary_excerpt, 80), ''),
                NULLIF(split_part(split_part(khi.url, '://', 2), '/', 1), ''),
                b.target_item_key)
FROM knowledge_trail_branches b
LEFT JOIN knowledge_home_items khi
  ON khi.user_id = b.user_id
  AND khi.item_key = b.target_item_key
  AND khi.projection_version = ` + activeProjectionVersionSQL + `
WHERE b.user_id = $1 AND b.state = 'open'
ORDER BY b.created_at DESC, b.branch_key DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("GetOpenTrailBranches: %w", err)
	}
	defer rows.Close()

	var branches []TrailBranch
	for rows.Next() {
		var b TrailBranch
		var refsJSON []byte
		if err := rows.Scan(&b.BranchKey, &b.AnchorItemKey, &b.RelationKind, &b.Why,
			&refsJSON, &b.Confidence, &b.TargetItemKey, &b.TargetTitle); err != nil {
			return nil, fmt.Errorf("GetOpenTrailBranches scan: %w", err)
		}
		unmarshalJSONWarn(refsJSON, &b.EvidenceRefs, "evidence_refs_json")
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// GetLatestFootprintAnchor returns the user's most recent footprint item_key and
// tenant — the spine point a freshly proposed branch forks from.
func (r *Repository) GetLatestFootprintAnchor(ctx context.Context, userID uuid.UUID) (itemKey string, tenantID uuid.UUID, ok bool, err error) {
	const q = `SELECT item_key, tenant_id FROM knowledge_trail_footprints
		WHERE user_id = $1 ORDER BY occurred_at DESC, footprint_key DESC LIMIT 1`
	row := r.pool.QueryRow(ctx, q, userID)
	if scanErr := row.Scan(&itemKey, &tenantID); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return "", uuid.Nil, false, nil
		}
		return "", uuid.Nil, false, fmt.Errorf("GetLatestFootprintAnchor: %w", scanErr)
	}
	return itemKey, tenantID, true, nil
}

// DeriveTrailClusterCandidates finds articles that share a tag with the user's
// engaged items but that the user has not footprinted — Cluster branch material.
// Ranked by tag-overlap. Producer-side derivation (the planner reads current
// state to decide what to emit); the projector that folds the resulting event
// stays payload-only.
func (r *Repository) DeriveTrailClusterCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]TrailClusterCandidate, error) {
	q := `
WITH active_version AS (
  SELECT ` + activeProjectionVersionSQL + ` AS v
),
user_tags AS (
  SELECT DISTINCT lower(t.tag) AS tag
  FROM knowledge_trail_footprints f
  JOIN knowledge_home_items khi
    ON khi.user_id = f.user_id AND khi.item_key = f.item_key
   AND khi.projection_version = (SELECT v FROM active_version)
  CROSS JOIN LATERAL jsonb_array_elements_text(khi.tags_json) AS t(tag)
  WHERE f.user_id = $1
),
footprinted AS (
  SELECT DISTINCT item_key FROM knowledge_trail_footprints WHERE user_id = $1
)
SELECT khi.item_key, khi.title,
       array_agg(DISTINCT it.tag) AS shared_tags
FROM knowledge_home_items khi
CROSS JOIN LATERAL jsonb_array_elements_text(khi.tags_json) AS it(tag)
WHERE khi.user_id = $1
  AND khi.item_type = 'article'
  AND khi.dismissed_at IS NULL
  AND khi.projection_version = (SELECT v FROM active_version)
  AND coalesce(khi.title, '') <> ''
  AND khi.item_key NOT IN (SELECT item_key FROM footprinted)
  AND lower(it.tag) IN (SELECT tag FROM user_tags)
GROUP BY khi.item_key, khi.title
ORDER BY count(DISTINCT lower(it.tag)) DESC, khi.item_key
LIMIT $2`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("DeriveTrailClusterCandidates: %w", err)
	}
	defer rows.Close()

	var out []TrailClusterCandidate
	for rows.Next() {
		var c TrailClusterCandidate
		if err := rows.Scan(&c.TargetItemKey, &c.TargetTitle, &c.SharedTags); err != nil {
			return nil, fmt.Errorf("DeriveTrailClusterCandidates scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
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
