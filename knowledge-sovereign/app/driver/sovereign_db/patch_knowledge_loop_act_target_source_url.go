package sovereign_db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// patchKnowledgeLoopActTargetSourceURLQuery patches act_targets[0].source_url
// on an existing entry row. Used by the corrective ArticleUrlBackfilled
// projector branch (a-la PatchKnowledgeHomeItemURL) to fill the external
// HTTPS URL on legacy projection rows whose seed event predated producer-side
// URL injection (ADR-000879 producer change).
//
// Reproject-safety / merge-safety:
//   - Single-key JSONB patch via jsonb_set(..., create_missing=true). The
//     rest of act_targets[0] (target_type, target_ref, route) and any other
//     elements stay byte-identical. dismiss_state, why_*, freshness_at,
//     surface_bucket, projection_revision, etc. are NOT in the SET clause —
//     a structural test in the matching _test.go can later assert no other
//     column name appears so a future edit cannot regress this.
//   - WHERE clause filters by (user_id, tenant_id, lens_mode_id, entry_key)
//     so the patch lands on exactly one row at a time, matching the per-user
//     scoping the projector relies on.
//   - `act_targets->0->>'target_type' = 'article'` predicate skips entries
//     whose first target is non-article (e.g. recap-only): the projector
//     orders article first when present, so this is also the correct guard.
//   - `act_targets->0->>'target_ref' = $5` ensures the URL maps to the
//     same article id that the projector decoded from the event payload —
//     a corrective event that drifted off the row's article cannot smuggle
//     an arbitrary URL onto a different entry.
//   - `NOT (act_targets->0 ? 'source_url')` keeps the patch idempotent and
//     conservative: once an entry has a URL, replays / out-of-order events
//     cannot overwrite it. A new ArticleCreated path that already populates
//     source_url is also untouched.
//   - `$6 <> ”` rejects empty-URL patches at the SQL boundary; the projector
//     also defends in Go via http/https allowlist (defense-in-depth).
//   - No SQL CASE / business judgement: the predicates are existence checks
//     and equality, not classification logic. business decisions stay in Go.
//   - projection_seq_hiwater is intentionally NOT bumped here. The patch is
//     a one-shot recovery and the JSONB existence guard already provides
//     replay idempotency. Bumping the hi-watermark would risk shadowing
//     forward-progressing events that haven't been seen yet on this row.
const patchKnowledgeLoopActTargetSourceURLQuery = `
UPDATE knowledge_loop_entries
SET act_targets = jsonb_set(act_targets, '{0,source_url}', to_jsonb($6::text), true)
WHERE user_id = $1
  AND tenant_id = $2
  AND lens_mode_id = $3
  AND entry_key = $4
  AND act_targets IS NOT NULL
  AND act_targets->0->>'target_type' = 'article'
  AND act_targets->0->>'target_ref' = $5
  AND NOT (act_targets->0 ? 'source_url')
  AND $6 <> ''
RETURNING projection_revision, projection_seq_hiwater
`

// PatchKnowledgeLoopActTargetSourceURL fills act_targets[0].source_url on a
// single Loop entry row. Returns SkippedBySeqHiwater when the row is missing,
// the source_url is already populated, or any other guard rejects — these
// are all safe no-op outcomes for an idempotent corrective event.
//
// Use this method only from the corrective ArticleUrlBackfilled projector
// branch; everything else MUST go through UpsertKnowledgeLoopEntry to
// preserve the seq-hiwater monotonicity invariant.
func (r *Repository) PatchKnowledgeLoopActTargetSourceURL(
	ctx context.Context,
	userID, tenantID, lensModeID, entryKey, articleID, sourceURL string,
	eventSeq int64,
) (*KnowledgeLoopUpsertResult, error) {
	if sourceURL == "" {
		return nil, errors.New("sovereign_db: empty source_url for patch")
	}
	if articleID == "" {
		return nil, errors.New("sovereign_db: empty article_id for patch")
	}
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, patchKnowledgeLoopActTargetSourceURLQuery,
		uID, tID, lensModeID, entryKey, articleID, sourceURL,
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("patch knowledge_loop_entries.act_targets.source_url: %w", err)
	}
	_ = eventSeq // intentionally unused: idempotency comes from the JSONB existence guard, not seq-hiwater
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}
