package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// tsToTime converts a proto Timestamp to a Go time.Time, treating a nil Timestamp as the zero value.
// Used for non-optional timestamp columns where a nil Timestamp is a programming error,
// but we do not want the projector to crash — the DB-side NOT NULL CHECK catches it later.
func tsToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// tsPtrToTime converts an optional proto Timestamp to a *time.Time so NULL can round-trip.
func tsPtrToTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// Knowledge Loop write path (ADR-000831). All UPSERTs enforce the seq-hiwater guard
// so out-of-order replay is a no-op; supersede pointer and artifact version IDs use
// COALESCE merge-safe semantics so a later non-supersede event cannot clear them.
// State-machine monotonicity (dismiss_state) is carried by the seq-hiwater guard,
// not by SQL CASE (per feedback_no_sql_logic).

const upsertKnowledgeLoopEntryQuery = `
INSERT INTO knowledge_loop_entries (
  user_id, tenant_id, lens_mode_id, entry_key, source_item_key,
  proposed_stage, surface_bucket,
  projection_revision, projection_seq_hiwater, source_event_seq,
  freshness_at, source_observed_at,
  artifact_summary_version_id, artifact_tag_set_version_id, artifact_lens_version_id,
  why_kind, why_text, why_confidence, why_evidence_ref_ids, why_evidence_refs,
  change_summary, continue_context, decision_options, act_targets,
  superseded_by_entry_key, dismiss_state, render_depth_hint, loop_priority
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7,
  1, $8, $9,
  $10, $11,
  $12, $13, $14,
  $15, $16, $17, $18, $19,
  $20, $21, $22, $23,
  $24, $25, $26, $27
)
ON CONFLICT (user_id, lens_mode_id, entry_key) DO UPDATE SET
  proposed_stage         = EXCLUDED.proposed_stage,
  surface_bucket         = EXCLUDED.surface_bucket,
  projection_revision    = knowledge_loop_entries.projection_revision + 1,
  projection_seq_hiwater = GREATEST(knowledge_loop_entries.projection_seq_hiwater, EXCLUDED.projection_seq_hiwater),
  source_event_seq       = EXCLUDED.source_event_seq,
  freshness_at           = GREATEST(knowledge_loop_entries.freshness_at, EXCLUDED.freshness_at),
  source_observed_at     = COALESCE(EXCLUDED.source_observed_at, knowledge_loop_entries.source_observed_at),
  projected_at           = NOW(),
  artifact_summary_version_id = COALESCE(EXCLUDED.artifact_summary_version_id, knowledge_loop_entries.artifact_summary_version_id),
  artifact_tag_set_version_id = COALESCE(EXCLUDED.artifact_tag_set_version_id, knowledge_loop_entries.artifact_tag_set_version_id),
  artifact_lens_version_id    = COALESCE(EXCLUDED.artifact_lens_version_id,    knowledge_loop_entries.artifact_lens_version_id),
  why_kind               = EXCLUDED.why_kind,
  why_text               = EXCLUDED.why_text,
  why_confidence         = EXCLUDED.why_confidence,
  why_evidence_ref_ids   = EXCLUDED.why_evidence_ref_ids,
  why_evidence_refs      = EXCLUDED.why_evidence_refs,
  change_summary         = COALESCE(EXCLUDED.change_summary,   knowledge_loop_entries.change_summary),
  continue_context       = COALESCE(EXCLUDED.continue_context, knowledge_loop_entries.continue_context),
  decision_options       = COALESCE(EXCLUDED.decision_options, knowledge_loop_entries.decision_options),
  act_targets            = COALESCE(EXCLUDED.act_targets,      knowledge_loop_entries.act_targets),
  superseded_by_entry_key = COALESCE(EXCLUDED.superseded_by_entry_key, knowledge_loop_entries.superseded_by_entry_key),
  dismiss_state          = EXCLUDED.dismiss_state,
  render_depth_hint      = EXCLUDED.render_depth_hint,
  loop_priority          = EXCLUDED.loop_priority
WHERE knowledge_loop_entries.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// KnowledgeLoopUpsertResult reports the outcome of a single UPSERT.
type KnowledgeLoopUpsertResult struct {
	Applied              bool
	SkippedBySeqHiwater  bool
	ProjectionRevision   int64
	ProjectionSeqHiwater int64
}

// UpsertKnowledgeLoopEntry inserts or updates a Knowledge Loop entry.
// Reproject-safe: old seq events return SkippedBySeqHiwater=true via the WHERE guard.
func (r *Repository) UpsertKnowledgeLoopEntry(
	ctx context.Context,
	e *sovereignv1.KnowledgeLoopEntry,
) (*KnowledgeLoopUpsertResult, error) {
	if e == nil {
		return nil, errors.New("sovereign_db: nil KnowledgeLoopEntry")
	}
	userID, err := uuid.Parse(e.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(e.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}
	freshnessAt := tsToTime(e.FreshnessAt)
	sourceObservedAt := tsPtrToTime(e.SourceObservedAt)

	artifact := e.ArtifactVersionRef
	var summaryVer, tagVer, lensVer *string
	if artifact != nil {
		summaryVer = artifact.SummaryVersionId
		tagVer = artifact.TagSetVersionId
		lensVer = artifact.LensVersionId
	}

	why := e.WhyPrimary
	whyKind := "source_why"
	whyText := ""
	var whyConfidence *float32
	if why != nil {
		whyKind = whyKindToDB(why.Kind)
		whyText = why.Text
		whyConfidence = why.Confidence
	}
	whyEvidenceRefIDs := make([]string, 0)
	whyEvidenceRefsJSON := []byte("[]")
	if why != nil && len(why.EvidenceRefs) > 0 {
		arr := make([]map[string]string, 0, len(why.EvidenceRefs))
		for _, r := range why.EvidenceRefs {
			whyEvidenceRefIDs = append(whyEvidenceRefIDs, r.RefId)
			arr = append(arr, map[string]string{"ref_id": r.RefId, "label": r.Label})
		}
		whyEvidenceRefsJSON, _ = json.Marshal(arr)
	}

	var supersededBy interface{}
	if e.SupersededByEntryKey != nil {
		supersededBy = *e.SupersededByEntryKey
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopEntryQuery,
		userID, tenantID, e.LensModeId, e.EntryKey, e.SourceItemKey,
		loopStageToDB(e.ProposedStage), surfaceBucketToDB(e.SurfaceBucket),
		e.ProjectionSeqHiwater, e.SourceEventSeq,
		freshnessAt, sourceObservedAt,
		summaryVer, tagVer, lensVer,
		whyKind, whyText, whyConfidence, whyEvidenceRefIDs, whyEvidenceRefsJSON,
		e.ChangeSummary, e.ContinueContext, e.DecisionOptions, e.ActTargets,
		supersededBy, dismissStateToDB(e.DismissState), int16(e.RenderDepthHint), loopPriorityToDB(e.LoopPriority),
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_entry: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

// patchKnowledgeLoopEntryWhyQuery updates only the why_* columns of an
// existing entry row. ADR-000846: the SummaryNarrativeBackfilled discovered
// event repairs historic entries whose original SummaryVersionCreated event
// lacked article_title in payload. This SQL preserves dismiss_state,
// freshness_at, surface_bucket, proposed_stage and every other field the
// full UPSERT would have overwritten — only the why narrative is patched.
//
// Reproject-safety:
//   - WHERE projection_seq_hiwater <= $5 makes idle replay a no-op once a
//     newer patch event has already landed.
//   - The patch event's effects converge across replay orders: the original
//     SummaryVersionCreated (lower seq) creates the entry with the fallback
//     narrative; the patch (higher seq) overwrites why_text only.
const patchKnowledgeLoopEntryWhyQuery = `
UPDATE knowledge_loop_entries SET
  projection_revision    = projection_revision + 1,
  projection_seq_hiwater = GREATEST(projection_seq_hiwater, $5),
  source_event_seq       = GREATEST(source_event_seq, $5),
  projected_at           = NOW(),
  why_kind               = $6,
  why_text               = $7,
  why_confidence         = $8,
  why_evidence_ref_ids   = $9,
  why_evidence_refs      = $10
WHERE user_id = $1 AND tenant_id = $2 AND lens_mode_id = $3 AND entry_key = $4
  AND projection_seq_hiwater <= $5
RETURNING projection_revision, projection_seq_hiwater
`

// PatchKnowledgeLoopEntryWhy patches only the why_* columns of an entry row,
// preserving dismiss_state and every other field. Returns SkippedBySeqHiwater
// if the row is missing OR the seq guard rejects (both safe outcomes — the
// next reproject will converge).
func (r *Repository) PatchKnowledgeLoopEntryWhy(
	ctx context.Context,
	userID, tenantID, lensModeID, entryKey string,
	eventSeq int64,
	why *sovereignv1.KnowledgeLoopWhyPayload,
) (*KnowledgeLoopUpsertResult, error) {
	if why == nil {
		return nil, errors.New("sovereign_db: nil WhyPayload for patch")
	}
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}

	whyKind := whyKindToDB(why.Kind)
	var whyConfidence *float32
	if why.Confidence != nil {
		whyConfidence = why.Confidence
	}
	whyEvidenceRefIDs := make([]string, 0, len(why.EvidenceRefs))
	whyEvidenceRefsJSON := []byte("[]")
	if len(why.EvidenceRefs) > 0 {
		arr := make([]map[string]string, 0, len(why.EvidenceRefs))
		for _, r := range why.EvidenceRefs {
			whyEvidenceRefIDs = append(whyEvidenceRefIDs, r.RefId)
			arr = append(arr, map[string]string{"ref_id": r.RefId, "label": r.Label})
		}
		whyEvidenceRefsJSON, _ = json.Marshal(arr)
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, patchKnowledgeLoopEntryWhyQuery,
		uID, tID, lensModeID, entryKey, eventSeq,
		whyKind, why.Text, whyConfidence, whyEvidenceRefIDs, whyEvidenceRefsJSON,
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("patch knowledge_loop_entry why: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

// patchKnowledgeLoopEntryDismissStateQuery flips the dismiss_state column of
// an existing entry row to the supplied state (typically `deferred` for the
// canonical contract §8.2 "passive dismiss / snooze" path).
//
// Reproject-safety / merge-safety:
//   - Single-column UPDATE; why_text, freshness_at, surface_bucket, decision_options,
//     etc. are intentionally NOT in the SET clause. A structural test asserts no
//     other column name appears here so a future edit cannot regress this.
//   - WHERE projection_seq_hiwater <= $5 + the SET's GREATEST() bump enforces the
//     monotonic seq guard so out-of-order delivery / replay is idempotent.
//   - dismiss_state itself is monotonic in practice: the only writer that uses
//     this query is the Deferred branch with a constant DEFERRED state, and the
//     ACTIVE → DEFERRED transition does not regress.
const patchKnowledgeLoopEntryDismissStateQuery = `
UPDATE knowledge_loop_entries SET
  projection_revision    = projection_revision + 1,
  projection_seq_hiwater = GREATEST(projection_seq_hiwater, $5),
  source_event_seq       = GREATEST(source_event_seq, $5),
  projected_at           = NOW(),
  dismiss_state          = $6
WHERE user_id = $1 AND tenant_id = $2 AND lens_mode_id = $3 AND entry_key = $4
  AND projection_seq_hiwater <= $5
RETURNING projection_revision, projection_seq_hiwater
`

// PatchKnowledgeLoopEntryDismissState patches only the dismiss_state column of
// an entry row, preserving every other field. Returns SkippedBySeqHiwater if the
// row is missing OR the seq guard rejects (both safe — replay converges).
func (r *Repository) PatchKnowledgeLoopEntryDismissState(
	ctx context.Context,
	userID, tenantID, lensModeID, entryKey string,
	eventSeq int64,
	dismissState sovereignv1.DismissState,
) (*KnowledgeLoopUpsertResult, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, patchKnowledgeLoopEntryDismissStateQuery,
		uID, tID, lensModeID, entryKey, eventSeq,
		dismissStateToDB(dismissState),
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("patch knowledge_loop_entry dismiss_state: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

const upsertKnowledgeLoopSessionStateQuery = `
INSERT INTO knowledge_loop_session_state (
  user_id, tenant_id, lens_mode_id,
  current_stage, current_stage_entered_at,
  focused_entry_key, foreground_entry_key,
  last_observed_entry_key, last_oriented_entry_key, last_decided_entry_key,
  last_acted_entry_key, last_returned_entry_key, last_deferred_entry_key,
  projection_revision, projection_seq_hiwater
) VALUES (
  $1, $2, $3,
  $4, $5,
  $6, $7,
  $8, $9, $10,
  $11, $12, $13,
  1, $14
)
ON CONFLICT (user_id, lens_mode_id) DO UPDATE SET
  current_stage            = EXCLUDED.current_stage,
  current_stage_entered_at = EXCLUDED.current_stage_entered_at,
  focused_entry_key        = COALESCE(EXCLUDED.focused_entry_key,    knowledge_loop_session_state.focused_entry_key),
  foreground_entry_key     = COALESCE(EXCLUDED.foreground_entry_key, knowledge_loop_session_state.foreground_entry_key),
  last_observed_entry_key  = COALESCE(EXCLUDED.last_observed_entry_key, knowledge_loop_session_state.last_observed_entry_key),
  last_oriented_entry_key  = COALESCE(EXCLUDED.last_oriented_entry_key, knowledge_loop_session_state.last_oriented_entry_key),
  last_decided_entry_key   = COALESCE(EXCLUDED.last_decided_entry_key,  knowledge_loop_session_state.last_decided_entry_key),
  last_acted_entry_key     = COALESCE(EXCLUDED.last_acted_entry_key,    knowledge_loop_session_state.last_acted_entry_key),
  last_returned_entry_key  = COALESCE(EXCLUDED.last_returned_entry_key, knowledge_loop_session_state.last_returned_entry_key),
  last_deferred_entry_key  = COALESCE(EXCLUDED.last_deferred_entry_key, knowledge_loop_session_state.last_deferred_entry_key),
  projection_revision      = knowledge_loop_session_state.projection_revision + 1,
  projection_seq_hiwater   = GREATEST(knowledge_loop_session_state.projection_seq_hiwater, EXCLUDED.projection_seq_hiwater),
  projected_at             = NOW()
WHERE knowledge_loop_session_state.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// UpsertKnowledgeLoopSessionState writes session state. current_stage_entered_at
// MUST be set by the caller from the triggering event's occurred_at.
func (r *Repository) UpsertKnowledgeLoopSessionState(
	ctx context.Context,
	s *sovereignv1.KnowledgeLoopSessionState,
) (*KnowledgeLoopUpsertResult, error) {
	if s == nil {
		return nil, errors.New("sovereign_db: nil KnowledgeLoopSessionState")
	}
	userID, err := uuid.Parse(s.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(s.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopSessionStateQuery,
		userID, tenantID, s.LensModeId,
		loopStageToDB(s.CurrentStage), tsToTime(s.CurrentStageEnteredAt),
		s.FocusedEntryKey, s.ForegroundEntryKey,
		s.LastObservedEntryKey, s.LastOrientedEntryKey, s.LastDecidedEntryKey,
		s.LastActedEntryKey, s.LastReturnedEntryKey, s.LastDeferredEntryKey,
		s.ProjectionSeqHiwater,
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_session_state: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

const upsertKnowledgeLoopSurfaceQuery = `
INSERT INTO knowledge_loop_surfaces (
  user_id, tenant_id, lens_mode_id, surface_bucket,
  primary_entry_key, secondary_entry_keys,
  projection_revision, projection_seq_hiwater, freshness_at,
  service_quality, loop_health
) VALUES (
  $1, $2, $3, $4,
  $5, $6,
  1, $7, $8,
  $9, $10
)
ON CONFLICT (user_id, lens_mode_id, surface_bucket) DO UPDATE SET
  primary_entry_key      = COALESCE(EXCLUDED.primary_entry_key, knowledge_loop_surfaces.primary_entry_key),
  secondary_entry_keys   = EXCLUDED.secondary_entry_keys,
  projection_revision    = knowledge_loop_surfaces.projection_revision + 1,
  projection_seq_hiwater = GREATEST(knowledge_loop_surfaces.projection_seq_hiwater, EXCLUDED.projection_seq_hiwater),
  freshness_at           = GREATEST(knowledge_loop_surfaces.freshness_at, EXCLUDED.freshness_at),
  projected_at           = NOW(),
  service_quality        = EXCLUDED.service_quality,
  loop_health            = EXCLUDED.loop_health
WHERE knowledge_loop_surfaces.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// UpsertKnowledgeLoopSurface writes a per-bucket surface summary.
func (r *Repository) UpsertKnowledgeLoopSurface(
	ctx context.Context,
	s *sovereignv1.KnowledgeLoopSurface,
) (*KnowledgeLoopUpsertResult, error) {
	if s == nil {
		return nil, errors.New("sovereign_db: nil KnowledgeLoopSurface")
	}
	userID, err := uuid.Parse(s.UserId)
	if err != nil {
		return nil, fmt.Errorf("parse user_id: %w", err)
	}
	tenantID, err := uuid.Parse(s.TenantId)
	if err != nil {
		return nil, fmt.Errorf("parse tenant_id: %w", err)
	}
	loopHealth := s.LoopHealth
	if len(loopHealth) == 0 {
		loopHealth = []byte("{}")
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopSurfaceQuery,
		userID, tenantID, s.LensModeId, surfaceBucketToDB(s.SurfaceBucket),
		s.PrimaryEntryKey, s.SecondaryEntryKeys,
		s.ProjectionSeqHiwater, tsToTime(s.FreshnessAt),
		serviceQualityToDB(s.ServiceQuality), loopHealth,
	)
	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_surface: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

const reserveKnowledgeLoopTransitionQuery = `
INSERT INTO knowledge_loop_transition_dedupes (user_id, client_transition_id)
VALUES ($1, $2)
ON CONFLICT (user_id, client_transition_id) DO NOTHING
RETURNING user_id
`

const loadCachedKnowledgeLoopTransitionQuery = `
SELECT canonical_entry_key, response_payload, created_at
FROM knowledge_loop_transition_dedupes
WHERE user_id = $1 AND client_transition_id = $2
`

// KnowledgeLoopReservationResult reports the outcome of a reservation.
type KnowledgeLoopReservationResult struct {
	Reserved            bool
	CanonicalEntryKey   *string
	ResponsePayloadJSON []byte
	CachedCreatedAt     *time.Time
}

// ReserveKnowledgeLoopTransition atomically claims an idempotency key. Returns
// Reserved=true on fresh claim; Reserved=false on duplicate, along with the
// cached response (if any) so the caller can replay it.
// Dedupe rows are ingest-only: reproject MUST NOT rebuild this table.
func (r *Repository) ReserveKnowledgeLoopTransition(
	ctx context.Context,
	userID uuid.UUID,
	clientTransitionID string,
) (*KnowledgeLoopReservationResult, error) {
	var claimed uuid.UUID
	err := r.pool.QueryRow(ctx, reserveKnowledgeLoopTransitionQuery, userID, clientTransitionID).Scan(&claimed)
	if err == nil {
		return &KnowledgeLoopReservationResult{Reserved: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("reserve knowledge_loop_transition: %w", err)
	}

	res := &KnowledgeLoopReservationResult{}
	scanErr := r.pool.QueryRow(ctx, loadCachedKnowledgeLoopTransitionQuery, userID, clientTransitionID).
		Scan(&res.CanonicalEntryKey, &res.ResponsePayloadJSON, &res.CachedCreatedAt)
	if scanErr != nil {
		return nil, fmt.Errorf("load cached knowledge_loop_transition: %w", scanErr)
	}
	return res, nil
}

// ---------- enum mappers (proto <-> DB enum string) ----------

func loopStageToDB(s sovereignv1.LoopStage) string {
	switch s {
	case sovereignv1.LoopStage_LOOP_STAGE_OBSERVE:
		return "observe"
	case sovereignv1.LoopStage_LOOP_STAGE_ORIENT:
		return "orient"
	case sovereignv1.LoopStage_LOOP_STAGE_DECIDE:
		return "decide"
	case sovereignv1.LoopStage_LOOP_STAGE_ACT:
		return "act"
	}
	return "observe"
}

func surfaceBucketToDB(b sovereignv1.SurfaceBucket) string {
	switch b {
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
		return "now"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE:
		return "continue"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
		return "changed"
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW:
		return "review"
	}
	return "now"
}

func dismissStateToDB(d sovereignv1.DismissState) string {
	switch d {
	case sovereignv1.DismissState_DISMISS_STATE_ACTIVE:
		return "active"
	case sovereignv1.DismissState_DISMISS_STATE_DEFERRED:
		return "deferred"
	case sovereignv1.DismissState_DISMISS_STATE_DISMISSED:
		return "dismissed"
	case sovereignv1.DismissState_DISMISS_STATE_COMPLETED:
		return "completed"
	}
	return "active"
}

func whyKindToDB(k sovereignv1.WhyKind) string {
	switch k {
	case sovereignv1.WhyKind_WHY_KIND_SOURCE:
		return "source_why"
	case sovereignv1.WhyKind_WHY_KIND_PATTERN:
		return "pattern_why"
	case sovereignv1.WhyKind_WHY_KIND_RECALL:
		return "recall_why"
	case sovereignv1.WhyKind_WHY_KIND_CHANGE:
		return "change_why"
	}
	return "source_why"
}

func loopPriorityToDB(p sovereignv1.LoopPriority) string {
	switch p {
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL:
		return "critical"
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CONTINUING:
		return "continuing"
	case sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM:
		return "confirm"
	case sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE:
		return "reference"
	}
	return "reference"
}

func serviceQualityToDB(q sovereignv1.LoopServiceQuality) string {
	switch q {
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL:
		return "full"
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_DEGRADED:
		return "degraded"
	case sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FALLBACK:
		return "fallback"
	}
	return "full"
}
