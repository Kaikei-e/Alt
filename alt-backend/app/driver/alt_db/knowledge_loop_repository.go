package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeLoopRepository implements read/write ports for the Knowledge Loop projection tables.
// It enforces the seq-hiwater guard on UPDATE so out-of-order event replay cannot overwrite
// newer projection state. Reads go through the knowledge_loop_entries_public view, which
// intentionally excludes projected_at.
type KnowledgeLoopRepository struct {
	pool PgxIface
}

// NewKnowledgeLoopRepository constructs a KnowledgeLoopRepository.
func NewKnowledgeLoopRepository(pool PgxIface) *KnowledgeLoopRepository {
	if pool == nil {
		return nil
	}
	return &KnowledgeLoopRepository{pool: pool}
}

// Compile-time interface assertions.
var (
	_ knowledge_loop_port.UpsertKnowledgeLoopEntryPort        = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.UpsertKnowledgeLoopSurfacePort      = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.ReserveTransitionIdempotencyPort    = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopEntriesPort         = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopSessionStatePort    = (*KnowledgeLoopRepository)(nil)
	_ knowledge_loop_port.GetKnowledgeLoopSurfacesPort        = (*KnowledgeLoopRepository)(nil)
)

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
  -- source_observed_at is monotonic-newest: once observed, do not regress to NULL on a later event
  source_observed_at     = COALESCE(EXCLUDED.source_observed_at, knowledge_loop_entries.source_observed_at),
  projected_at           = NOW(),
  -- Artifact version refs are monotonic-latest per kind: once a version id is known, do not lose it
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
  -- Supersede pointer is monotonic-set: once we learn the successor, a later non-supersede
  -- event MUST NOT clear it (per /rules/knowledge-home.md merge-safe upsert rule).
  -- COALESCE here is a nil-skip merge pattern, not business logic — it only chooses between
  -- "new value present" and "existing value". Dismiss-state monotonicity is enforced upstream
  -- by the seq_hiwater guard (old events cannot overwrite newer rows), so the projector
  -- produces events in a non-regressing order and no SQL CASE is required.
  superseded_by_entry_key = COALESCE(EXCLUDED.superseded_by_entry_key, knowledge_loop_entries.superseded_by_entry_key),
  dismiss_state          = EXCLUDED.dismiss_state,
  render_depth_hint      = EXCLUDED.render_depth_hint,
  loop_priority          = EXCLUDED.loop_priority
WHERE knowledge_loop_entries.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// UpsertKnowledgeLoopEntry inserts or revises a Loop entry row.
// If the existing row has a newer projection_seq_hiwater, the update is a no-op and
// SkippedBySeqHiwater is true.
func (r *KnowledgeLoopRepository) UpsertKnowledgeLoopEntry(
	ctx context.Context,
	entry *domain.KnowledgeLoopEntry,
) (*knowledge_loop_port.UpsertResult, error) {
	if entry == nil {
		return nil, errors.New("knowledge_loop_repository: nil entry")
	}

	whyEvidenceRefsJSON, err := json.Marshal(entry.WhyEvidenceRefs)
	if err != nil {
		return nil, fmt.Errorf("marshal why_evidence_refs: %w", err)
	}
	if len(entry.WhyEvidenceRefs) == 0 {
		whyEvidenceRefsJSON = []byte("[]")
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopEntryQuery,
		entry.UserID, entry.TenantID, entry.LensModeID, entry.EntryKey, entry.SourceItemKey,
		string(entry.ProposedStage), string(entry.SurfaceBucket),
		entry.ProjectionSeqHiwater, entry.SourceEventSeq,
		entry.FreshnessAt, entry.SourceObservedAt,
		entry.ArtifactVersionRef.SummaryVersionID, entry.ArtifactVersionRef.TagSetVersionID, entry.ArtifactVersionRef.LensVersionID,
		string(entry.WhyKind), entry.WhyText, entry.WhyConfidence, entry.WhyEvidenceRefIDs, whyEvidenceRefsJSON,
		entry.ChangeSummary, entry.ContinueContext, entry.DecisionOptions, entry.ActTargets,
		entry.SupersededByEntryKey, string(entry.DismissState), int16(entry.RenderDepthHint), string(entry.LoopPriority),
	)

	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ON CONFLICT DO UPDATE ... WHERE false: no row returned → guard rejected stale seq.
			return &knowledge_loop_port.UpsertResult{
				Applied:             false,
				SkippedBySeqHiwater: true,
			}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_entry: %w", err)
	}

	return &knowledge_loop_port.UpsertResult{
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
  focused_entry_key        = EXCLUDED.focused_entry_key,
  foreground_entry_key     = EXCLUDED.foreground_entry_key,
  last_observed_entry_key  = EXCLUDED.last_observed_entry_key,
  last_oriented_entry_key  = EXCLUDED.last_oriented_entry_key,
  last_decided_entry_key   = EXCLUDED.last_decided_entry_key,
  last_acted_entry_key     = EXCLUDED.last_acted_entry_key,
  last_returned_entry_key  = EXCLUDED.last_returned_entry_key,
  last_deferred_entry_key  = EXCLUDED.last_deferred_entry_key,
  projection_revision      = knowledge_loop_session_state.projection_revision + 1,
  projection_seq_hiwater   = GREATEST(knowledge_loop_session_state.projection_seq_hiwater, EXCLUDED.projection_seq_hiwater),
  projected_at             = NOW()
WHERE knowledge_loop_session_state.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// UpsertKnowledgeLoopSessionState writes session state with seq-hiwater guard.
// current_stage_entered_at MUST be set by the caller from the triggering event's occurred_at.
func (r *KnowledgeLoopRepository) UpsertKnowledgeLoopSessionState(
	ctx context.Context,
	state *domain.KnowledgeLoopSessionState,
) (*knowledge_loop_port.UpsertResult, error) {
	if state == nil {
		return nil, errors.New("knowledge_loop_repository: nil state")
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopSessionStateQuery,
		state.UserID, state.TenantID, state.LensModeID,
		string(state.CurrentStage), state.CurrentStageEnteredAt,
		state.FocusedEntryKey, state.ForegroundEntryKey,
		state.LastObservedEntryKey, state.LastOrientedEntryKey, state.LastDecidedEntryKey,
		state.LastActedEntryKey, state.LastReturnedEntryKey, state.LastDeferredEntryKey,
		state.ProjectionSeqHiwater,
	)

	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &knowledge_loop_port.UpsertResult{
				Applied:             false,
				SkippedBySeqHiwater: true,
			}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_session_state: %w", err)
	}
	return &knowledge_loop_port.UpsertResult{
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
  primary_entry_key      = EXCLUDED.primary_entry_key,
  secondary_entry_keys   = EXCLUDED.secondary_entry_keys,
  projection_revision    = knowledge_loop_surfaces.projection_revision + 1,
  projection_seq_hiwater = GREATEST(knowledge_loop_surfaces.projection_seq_hiwater, EXCLUDED.projection_seq_hiwater),
  freshness_at           = EXCLUDED.freshness_at,
  projected_at           = NOW(),
  service_quality        = EXCLUDED.service_quality,
  loop_health            = EXCLUDED.loop_health
WHERE knowledge_loop_surfaces.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater
RETURNING projection_revision, projection_seq_hiwater
`

// UpsertKnowledgeLoopSurface writes a per-bucket surface summary with seq-hiwater guard.
func (r *KnowledgeLoopRepository) UpsertKnowledgeLoopSurface(
	ctx context.Context,
	s *domain.KnowledgeLoopSurface,
) (*knowledge_loop_port.UpsertResult, error) {
	if s == nil {
		return nil, errors.New("knowledge_loop_repository: nil surface")
	}

	loopHealth := s.LoopHealth
	if len(loopHealth) == 0 {
		loopHealth = []byte("{}")
	}

	var revision, seqHiwater int64
	row := r.pool.QueryRow(ctx, upsertKnowledgeLoopSurfaceQuery,
		s.UserID, s.TenantID, s.LensModeID, string(s.SurfaceBucket),
		s.PrimaryEntryKey, s.SecondaryEntryKeys,
		s.ProjectionSeqHiwater, s.FreshnessAt,
		string(s.ServiceQuality), loopHealth,
	)

	if err := row.Scan(&revision, &seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &knowledge_loop_port.UpsertResult{
				Applied:             false,
				SkippedBySeqHiwater: true,
			}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_surface: %w", err)
	}
	return &knowledge_loop_port.UpsertResult{
		Applied:              true,
		ProjectionRevision:   revision,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

const reserveTransitionIdempotencyQuery = `
INSERT INTO knowledge_loop_transition_dedupes (user_id, client_transition_id)
VALUES ($1, $2)
ON CONFLICT (user_id, client_transition_id) DO NOTHING
RETURNING user_id
`

const loadCachedTransitionResponseQuery = `
SELECT canonical_entry_key, response_payload, created_at
FROM knowledge_loop_transition_dedupes
WHERE user_id = $1 AND client_transition_id = $2
`

// ReserveTransitionIdempotency returns (true, nil, nil) on fresh claim and
// (false, cached, nil) on duplicate. The table is ingest-only: reproject leaves it alone.
func (r *KnowledgeLoopRepository) ReserveTransitionIdempotency(
	ctx context.Context,
	userID uuid.UUID,
	clientTransitionID string,
) (bool, *knowledge_loop_port.CachedTransitionResponse, error) {
	var claimed uuid.UUID
	err := r.pool.QueryRow(ctx, reserveTransitionIdempotencyQuery, userID, clientTransitionID).Scan(&claimed)
	if err == nil {
		return true, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, nil, fmt.Errorf("reserve transition idempotency: %w", err)
	}

	cached := &knowledge_loop_port.CachedTransitionResponse{}
	if scanErr := r.pool.QueryRow(ctx, loadCachedTransitionResponseQuery, userID, clientTransitionID).
		Scan(&cached.CanonicalEntryKey, &cached.ResponsePayloadJSON, &cached.CreatedAt); scanErr != nil {
		return false, nil, fmt.Errorf("load cached transition response: %w", scanErr)
	}
	return false, cached, nil
}

// GetKnowledgeLoopEntries is a placeholder; full read path lands in M3.
// Reads MUST go through knowledge_loop_entries_public to keep projected_at out of the caller.
func (r *KnowledgeLoopRepository) GetKnowledgeLoopEntries(
	ctx context.Context,
	q knowledge_loop_port.GetEntriesQuery,
) ([]*domain.KnowledgeLoopEntry, error) {
	return nil, errors.New("knowledge_loop_repository: GetKnowledgeLoopEntries not yet implemented (M3)")
}

// GetKnowledgeLoopSessionState is a placeholder for M3.
func (r *KnowledgeLoopRepository) GetKnowledgeLoopSessionState(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) (*domain.KnowledgeLoopSessionState, error) {
	return nil, errors.New("knowledge_loop_repository: GetKnowledgeLoopSessionState not yet implemented (M3)")
}

// GetKnowledgeLoopSurfaces is a placeholder for M3.
func (r *KnowledgeLoopRepository) GetKnowledgeLoopSurfaces(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) ([]*domain.KnowledgeLoopSurface, error) {
	return nil, errors.New("knowledge_loop_repository: GetKnowledgeLoopSurfaces not yet implemented (M3)")
}
