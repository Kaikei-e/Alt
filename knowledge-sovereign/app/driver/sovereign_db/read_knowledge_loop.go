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

// Knowledge Loop read path. All SELECTs go through knowledge_loop_entries_public
// (which excludes projected_at) so the caller cannot accidentally observe the
// internal debug wall-clock.

// GetKnowledgeLoopEntriesFilter scopes a read request.
type GetKnowledgeLoopEntriesFilter struct {
	TenantID         uuid.UUID
	UserID           uuid.UUID
	LensModeID       string
	SurfaceBucket    *sovereignv1.SurfaceBucket
	IncludeDismissed bool
	Limit            int
}

// GetKnowledgeLoopEntries returns entries matching the filter.
func (r *Repository) GetKnowledgeLoopEntries(
	ctx context.Context,
	f GetKnowledgeLoopEntriesFilter,
) ([]*sovereignv1.KnowledgeLoopEntry, error) {
	if f.LensModeID == "" {
		return nil, errors.New("sovereign_db: lens_mode_id is required")
	}

	args := []interface{}{f.UserID, f.TenantID, f.LensModeID}
	where := "e.user_id = $1 AND e.tenant_id = $2 AND e.lens_mode_id = $3"

	if !f.IncludeDismissed {
		where += " AND e.dismiss_state = 'active'"
	}
	if f.SurfaceBucket != nil {
		args = append(args, surfaceBucketToDB(*f.SurfaceBucket))
		where += fmt.Sprintf(" AND e.surface_bucket = $%d", len(args))
	}

	limit := f.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
SELECT
  e.user_id, e.tenant_id, e.lens_mode_id, e.entry_key, e.source_item_key,
  e.proposed_stage, e.surface_bucket,
  e.projection_revision, e.projection_seq_hiwater, e.source_event_seq,
  e.freshness_at, e.source_observed_at,
  e.artifact_summary_version_id, e.artifact_tag_set_version_id, e.artifact_lens_version_id,
  e.why_kind, e.why_text, e.why_confidence, e.why_evidence_ref_ids, e.why_evidence_refs,
  e.change_summary, e.continue_context, e.decision_options, e.act_targets,
  e.superseded_by_entry_key, e.dismiss_state, e.render_depth_hint, e.loop_priority,
  e.surface_planner_version, e.surface_score_inputs,
  s.current_stage, s.current_stage_entered_at
FROM knowledge_loop_entries_public e
LEFT JOIN knowledge_loop_entry_session_state s
  ON s.user_id = e.user_id
 AND s.tenant_id = e.tenant_id
 AND s.lens_mode_id = e.lens_mode_id
 AND s.entry_key = e.entry_key
WHERE %s
ORDER BY e.projection_seq_hiwater DESC
LIMIT $%d
`, where, len(args))

	var out []*sovereignv1.KnowledgeLoopEntry
	err := r.withUserContext(ctx, f.UserID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query knowledge_loop_entries: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			e, err := scanKnowledgeLoopEntry(rows)
			if err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scanKnowledgeLoopEntry(row pgx.Row) (*sovereignv1.KnowledgeLoopEntry, error) {
	var (
		userID, tenantID                          uuid.UUID
		lensModeID, entryKey, sourceItemKey       string
		proposedStage, surfaceBucket              string
		projectionRevision, seqHiwater, sourceSeq int64
		freshnessAt                               time.Time
		sourceObservedAt                          *time.Time
		artSummary, artTagSet, artLens            *string
		whyKind, whyText                          string
		whyConfidence                             *float32
		whyEvidenceRefIDs                         []string
		whyEvidenceRefsJSON                       []byte
		changeSummary, continueContext            []byte
		decisionOptions, actTargets               []byte
		supersededBy                              *string
		dismissState                              string
		renderDepth                               int16
		loopPriority                              string
		surfacePlannerVersion                     int16
		surfaceScoreInputs                        []byte
		currentEntryStage                         *string
		currentEntryStageEnteredAt                *time.Time
	)

	err := row.Scan(
		&userID, &tenantID, &lensModeID, &entryKey, &sourceItemKey,
		&proposedStage, &surfaceBucket,
		&projectionRevision, &seqHiwater, &sourceSeq,
		&freshnessAt, &sourceObservedAt,
		&artSummary, &artTagSet, &artLens,
		&whyKind, &whyText, &whyConfidence, &whyEvidenceRefIDs, &whyEvidenceRefsJSON,
		&changeSummary, &continueContext, &decisionOptions, &actTargets,
		&supersededBy, &dismissState, &renderDepth, &loopPriority,
		&surfacePlannerVersion, &surfaceScoreInputs,
		&currentEntryStage, &currentEntryStageEnteredAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan knowledge_loop_entry: %w", err)
	}

	e := &sovereignv1.KnowledgeLoopEntry{
		UserId:               userID.String(),
		TenantId:             tenantID.String(),
		LensModeId:           lensModeID,
		EntryKey:             entryKey,
		SourceItemKey:        sourceItemKey,
		ProposedStage:        loopStageFromDB(proposedStage),
		SurfaceBucket:        surfaceBucketFromDB(surfaceBucket),
		ProjectionRevision:   projectionRevision,
		ProjectionSeqHiwater: seqHiwater,
		SourceEventSeq:       sourceSeq,
		FreshnessAt:          timestampFromTime(freshnessAt),
		SourceObservedAt:     timestampFromTimePtr(sourceObservedAt),
		ArtifactVersionRef: &sovereignv1.KnowledgeLoopArtifactVersionRef{
			SummaryVersionId: artSummary,
			TagSetVersionId:  artTagSet,
			LensVersionId:    artLens,
		},
		WhyPrimary: &sovereignv1.KnowledgeLoopWhyPayload{
			Kind:       whyKindFromDB(whyKind),
			Text:       whyText,
			Confidence: whyConfidence,
		},
		ChangeSummary:         changeSummary,
		ContinueContext:       continueContext,
		DecisionOptions:       decisionOptions,
		ActTargets:            actTargets,
		SupersededByEntryKey:  supersededBy,
		DismissState:          dismissStateFromDB(dismissState),
		RenderDepthHint:       int32(renderDepth),
		LoopPriority:          loopPriorityFromDB(loopPriority),
		SurfacePlannerVersion: plannerVersionFromDB(surfacePlannerVersion).Enum(),
		SurfaceScoreInputs:    surfaceScoreInputs,
	}
	if currentEntryStage != nil {
		stage := loopStageFromDB(*currentEntryStage)
		e.CurrentEntryStage = &stage
	}
	if currentEntryStageEnteredAt != nil {
		e.CurrentEntryStageEnteredAt = timestampFromTimePtr(currentEntryStageEnteredAt)
	}

	if len(whyEvidenceRefsJSON) > 0 {
		var arr []map[string]string
		if err := json.Unmarshal(whyEvidenceRefsJSON, &arr); err == nil {
			for _, a := range arr {
				e.WhyPrimary.EvidenceRefs = append(e.WhyPrimary.EvidenceRefs, &sovereignv1.KnowledgeLoopEvidenceRef{
					RefId: a["ref_id"],
					Label: a["label"],
				})
			}
		}
	}
	return e, nil
}

// GetKnowledgeLoopSessionState fetches the per-user-per-lens session row.
func (r *Repository) GetKnowledgeLoopSessionState(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) (*sovereignv1.KnowledgeLoopSessionState, error) {
	const query = `
SELECT user_id, tenant_id, lens_mode_id,
       current_stage, current_stage_entered_at,
       focused_entry_key, foreground_entry_key,
       last_observed_entry_key, last_oriented_entry_key, last_decided_entry_key,
       last_acted_entry_key, last_returned_entry_key, last_deferred_entry_key,
       projection_revision, projection_seq_hiwater
FROM knowledge_loop_session_state
WHERE user_id = $1 AND tenant_id = $2 AND lens_mode_id = $3
`
	var (
		uid, tid                                             uuid.UUID
		lens, stage                                          string
		stageEnteredAt                                       time.Time
		focused, foreground                                  *string
		lastObs, lastOri, lastDec, lastAct, lastRet, lastDef *string
		rev, seq                                             int64
	)
	var noRows bool
	err := r.withUserContext(ctx, userID, func(tx pgx.Tx) error {
		scanErr := tx.QueryRow(ctx, query, userID, tenantID, lensModeID).Scan(
			&uid, &tid, &lens, &stage, &stageEnteredAt,
			&focused, &foreground,
			&lastObs, &lastOri, &lastDec, &lastAct, &lastRet, &lastDef,
			&rev, &seq,
		)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			noRows = true
			return nil
		}
		return scanErr
	})
	if noRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get knowledge_loop_session_state: %w", err)
	}
	return &sovereignv1.KnowledgeLoopSessionState{
		UserId:                uid.String(),
		TenantId:              tid.String(),
		LensModeId:            lens,
		CurrentStage:          loopStageFromDB(stage),
		CurrentStageEnteredAt: timestampFromTime(stageEnteredAt),
		FocusedEntryKey:       focused,
		ForegroundEntryKey:    foreground,
		LastObservedEntryKey:  lastObs,
		LastOrientedEntryKey:  lastOri,
		LastDecidedEntryKey:   lastDec,
		LastActedEntryKey:     lastAct,
		LastReturnedEntryKey:  lastRet,
		LastDeferredEntryKey:  lastDef,
		ProjectionRevision:    rev,
		ProjectionSeqHiwater:  seq,
	}, nil
}

// GetKnowledgeLoopSurfaces returns all surface buckets for a user/lens.
func (r *Repository) GetKnowledgeLoopSurfaces(
	ctx context.Context,
	tenantID, userID uuid.UUID,
	lensModeID string,
) ([]*sovereignv1.KnowledgeLoopSurface, error) {
	const query = `
SELECT user_id, tenant_id, lens_mode_id, surface_bucket,
       primary_entry_key, secondary_entry_keys,
       projection_revision, projection_seq_hiwater,
       freshness_at, service_quality, loop_health
FROM knowledge_loop_surfaces
WHERE user_id = $1 AND tenant_id = $2 AND lens_mode_id = $3
ORDER BY surface_bucket
`
	var out []*sovereignv1.KnowledgeLoopSurface
	err := r.withUserContext(ctx, userID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, userID, tenantID, lensModeID)
		if err != nil {
			return fmt.Errorf("query knowledge_loop_surfaces: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var (
				uid, tid     uuid.UUID
				lens, bucket string
				primary      *string
				secondary    []string
				rev, seq     int64
				freshness    time.Time
				quality      string
				health       []byte
			)
			if err := rows.Scan(
				&uid, &tid, &lens, &bucket,
				&primary, &secondary,
				&rev, &seq,
				&freshness, &quality, &health,
			); err != nil {
				return fmt.Errorf("scan knowledge_loop_surface: %w", err)
			}
			out = append(out, &sovereignv1.KnowledgeLoopSurface{
				UserId:               uid.String(),
				TenantId:             tid.String(),
				LensModeId:           lens,
				SurfaceBucket:        surfaceBucketFromDB(bucket),
				PrimaryEntryKey:      primary,
				SecondaryEntryKeys:   secondary,
				ProjectionRevision:   rev,
				ProjectionSeqHiwater: seq,
				FreshnessAt:          timestampFromTime(freshness),
				ServiceQuality:       serviceQualityFromDB(quality),
				LoopHealth:           health,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- inverse enum mappers ----------

func loopStageFromDB(s string) sovereignv1.LoopStage {
	switch s {
	case "observe":
		return sovereignv1.LoopStage_LOOP_STAGE_OBSERVE
	case "orient":
		return sovereignv1.LoopStage_LOOP_STAGE_ORIENT
	case "decide":
		return sovereignv1.LoopStage_LOOP_STAGE_DECIDE
	case "act":
		return sovereignv1.LoopStage_LOOP_STAGE_ACT
	}
	return sovereignv1.LoopStage_LOOP_STAGE_UNSPECIFIED
}

func surfaceBucketFromDB(b string) sovereignv1.SurfaceBucket {
	switch b {
	case "now":
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	case "continue":
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	case "changed":
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	case "review":
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}
	return sovereignv1.SurfaceBucket_SURFACE_BUCKET_UNSPECIFIED
}

func dismissStateFromDB(d string) sovereignv1.DismissState {
	switch d {
	case "active":
		return sovereignv1.DismissState_DISMISS_STATE_ACTIVE
	case "deferred":
		return sovereignv1.DismissState_DISMISS_STATE_DEFERRED
	case "dismissed":
		return sovereignv1.DismissState_DISMISS_STATE_DISMISSED
	case "completed":
		return sovereignv1.DismissState_DISMISS_STATE_COMPLETED
	}
	return sovereignv1.DismissState_DISMISS_STATE_UNSPECIFIED
}

func whyKindFromDB(k string) sovereignv1.WhyKind {
	switch k {
	case "source_why":
		return sovereignv1.WhyKind_WHY_KIND_SOURCE
	case "pattern_why":
		return sovereignv1.WhyKind_WHY_KIND_PATTERN
	case "recall_why":
		return sovereignv1.WhyKind_WHY_KIND_RECALL
	case "change_why":
		return sovereignv1.WhyKind_WHY_KIND_CHANGE
	case "topic_affinity_why":
		return sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY
	case "tag_trending_why":
		return sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING
	case "unfinished_continue_why":
		return sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE
	}
	return sovereignv1.WhyKind_WHY_KIND_UNSPECIFIED
}

func plannerVersionFromDB(v int16) sovereignv1.SurfacePlannerVersion {
	if v == 2 {
		return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2
	}
	return sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
}

func loopPriorityFromDB(p string) sovereignv1.LoopPriority {
	switch p {
	case "critical":
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL
	case "continuing":
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONTINUING
	case "confirm":
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM
	case "reference":
		return sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE
	}
	return sovereignv1.LoopPriority_LOOP_PRIORITY_UNSPECIFIED
}

func serviceQualityFromDB(q string) sovereignv1.LoopServiceQuality {
	switch q {
	case "full":
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL
	case "degraded":
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_DEGRADED
	case "fallback":
		return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FALLBACK
	}
	return sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_UNSPECIFIED
}

// timestampFromTime converts a time.Time into a proto Timestamp; zero time -> nil.
func timestampFromTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// timestampFromTimePtr handles nullable timestamptz columns.
func timestampFromTimePtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}
