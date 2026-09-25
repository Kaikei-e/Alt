package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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

// FootprintAnchor is the spine point a branch forks from: the item the why
// names, the tenant that owns it, and the verb that makes the why true.
type FootprintAnchor struct {
	ItemKey  string
	TenantID uuid.UUID
	Verb     string
}

func scanTrailBranchRow(scanner rowScanner) (TrailBranch, error) {
	var b TrailBranch
	var refsJSON []byte
	if err := scanner.Scan(&b.BranchKey, &b.AnchorItemKey, &b.RelationKind, &b.Why,
		&refsJSON, &b.Confidence, &b.TargetItemKey, &b.TargetTitle); err != nil {
		return TrailBranch{}, err
	}
	unmarshalJSONWarn(refsJSON, &b.EvidenceRefs, "evidence_refs_json")
	return b, nil
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
		b, err := scanTrailBranchRow(rows)
		if err != nil {
			return nil, fmt.Errorf("GetOpenTrailBranches scan: %w", err)
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// GetOpenTrailBranchesForAnchor returns the user's open branches anchored on
// one item, newest first, capped at limit (Wave 10, D26 — the patch-exit
// surface is a handful, not an inbox). Mirrors GetOpenTrailBranches with an
// anchor filter and a server-side cap.
func (r *Repository) GetOpenTrailBranchesForAnchor(ctx context.Context, userID uuid.UUID, anchorItemKey string, limit int) ([]TrailBranch, error) {
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
WHERE b.user_id = $1 AND b.anchor_item_key = $2 AND b.state = 'open'
ORDER BY b.created_at DESC, b.branch_key DESC
LIMIT $3`
	rows, err := r.pool.Query(ctx, q, userID, anchorItemKey, limit)
	if err != nil {
		return nil, fmt.Errorf("GetOpenTrailBranchesForAnchor: %w", err)
	}
	defer rows.Close()

	var branches []TrailBranch
	for rows.Next() {
		b, err := scanTrailBranchRow(rows)
		if err != nil {
			return nil, fmt.Errorf("GetOpenTrailBranchesForAnchor scan: %w", err)
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

// GetItemTitle resolves one item's live display title (D28 — anchored why):
// the trail planner uses it to name the anchor a branch's why must reference.
// ok=false means the title is absent or blank — the caller must not fabricate
// a why around an item it cannot name.
func (r *Repository) GetItemTitle(ctx context.Context, userID uuid.UUID, itemKey string) (title string, ok bool, err error) {
	const q = `SELECT title FROM knowledge_home_items
		WHERE user_id = $1 AND item_key = $2 AND projection_version = ` + activeProjectionVersionSQL
	row := r.pool.QueryRow(ctx, q, userID, itemKey)
	if scanErr := row.Scan(&title); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("GetItemTitle: %w", scanErr)
	}
	return title, strings.TrimSpace(title) != "", nil
}

// GetLatestFootprintAnchor returns the user's most recent footprint that can
// anchor a branch — the spine point a freshly proposed branch forks from. Only
// EngagementVerbs qualify: the why must name the act that happened, and a
// dismissal cannot back "Because you read this" (core-concept §C4). The verb
// travels with the anchor so the caller phrases the why from it rather than
// assuming a read. ok=false means the spine holds nothing that can truthfully
// anchor a why — the caller must suppress, never invent one.
func (r *Repository) GetLatestFootprintAnchor(ctx context.Context, userID uuid.UUID) (FootprintAnchor, bool, error) {
	const q = `SELECT item_key, tenant_id, verb FROM knowledge_trail_footprints
		WHERE user_id = $1 AND verb = ANY($2::text[])
		ORDER BY occurred_at DESC, footprint_key DESC LIMIT 1`
	var a FootprintAnchor
	row := r.pool.QueryRow(ctx, q, userID, EngagementVerbs)
	if scanErr := row.Scan(&a.ItemKey, &a.TenantID, &a.Verb); scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return FootprintAnchor{}, false, nil
		}
		return FootprintAnchor{}, false, fmt.Errorf("GetLatestFootprintAnchor: %w", scanErr)
	}
	return a, true, nil
}
