package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// patchKnowledgeHomeItemURLQuery patches only the `url` column of an
// existing knowledge_home_items row, preserving every other column. Used
// by the corrective `ArticleUrlBackfilled` event projector branch (a-la
// ADR-000846's PatchKnowledgeLoopEntryWhy pattern) to repair article
// rows whose original `ArticleCreated` event was written with the legacy
// `"link"` wire key (or with no URL at all).
//
// Reproject-safety / merge-safety:
//   - Single-column UPDATE (plus updated_at). title, summary_excerpt,
//     why_json, score, etc. are intentionally NOT in the SET clause.
//     A structural test in the matching _test.go asserts no other column
//     name appears here so a future edit cannot regress this.
//   - The WHERE clause filters by (user_id, item_key, projection_version)
//     so the patch lands on exactly one row at a time and Reproject can
//     safely re-run it without touching live v_active during shadow build.
//   - `AND $1 != ”` rejects empty-URL patches at the SQL boundary; the
//     projector also defends in Go via URL scheme allowlist. Two layers
//     because the Go-side check is the canonical guard but a stray caller
//     using the driver directly would otherwise be able to wipe URLs.
//   - Idempotent: re-applying the same event yields the same row state
//     (URL is constant per event payload). No projection_seq_hiwater
//     column exists on knowledge_home_items, but the value stability
//     across replay makes one unnecessary.
const patchKnowledgeHomeItemURLQuery = `
UPDATE knowledge_home_items
SET url = $1,
    updated_at = NOW()
WHERE user_id = $2
  AND item_key = $3
  AND projection_version = $4
  AND $1 <> ''
`

// PatchKnowledgeHomeItemURLPayload is the JSON payload accepted on the
// Connect-RPC ApplyProjectionMutation envelope when MutationType ==
// MutationPatchHomeItemURL. The single source of truth for the wire
// schema lives in alt-backend's knowledge_sovereign_port package; this
// struct mirrors it for unmarshalling.
type PatchKnowledgeHomeItemURLPayload struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	URL               string `json:"url"`
}

// PatchKnowledgeHomeItemURL applies the URL patch to one row, identified
// by (user_id, item_key, projection_version). Empty URL is rejected at
// the WHERE clause, so a no-op UPDATE is a safe outcome (returned without
// error). Use this method only from the corrective ArticleUrlBackfilled
// projector branch; other callers MUST go through UpsertKnowledgeHomeItem.
func (r *Repository) PatchKnowledgeHomeItemURL(ctx context.Context, payload json.RawMessage) error {
	var p PatchKnowledgeHomeItemURLPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("PatchKnowledgeHomeItemURL: unmarshal: %w", err)
	}
	if p.URL == "" {
		// Reject at the application boundary so callers fail loudly
		// rather than silently emitting no-op UPDATEs.
		return errors.New("PatchKnowledgeHomeItemURL: empty URL")
	}
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return fmt.Errorf("PatchKnowledgeHomeItemURL: parse user_id: %w", err)
	}
	if _, err := r.pool.Exec(ctx, patchKnowledgeHomeItemURLQuery,
		p.URL, userID, p.ItemKey, p.ProjectionVersion,
	); err != nil {
		return fmt.Errorf("PatchKnowledgeHomeItemURL: %w", err)
	}
	return nil
}
