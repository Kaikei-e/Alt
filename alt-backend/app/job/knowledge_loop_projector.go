package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_loop_port"
	"alt/port/knowledge_projection_port"
	"alt/usecase/knowledge_loop_usecase"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	knowledgeLoopProjectorName = "knowledge-loop-projector"
	knowledgeLoopBatchSize     = 100
	defaultLoopLensModeID      = "default"
)

// KnowledgeLoopProjectorConfig configures the Knowledge Loop projector.
type KnowledgeLoopProjectorConfig struct {
	BatchSize int
}

// KnowledgeLoopProjectorJob returns a scheduler-compatible closure that reads a batch
// of knowledge_events and projects Loop rows.
//
// Reproject-safety invariants (see docs/plan/knowledge-loop-canonical-contract.md and ADR-000831):
//   - Reads only event payloads. Never reads latest projection state.
//   - freshness_at and current_stage_entered_at come from event.occurred_at, never wall-clock.
//   - UPSERTs enforce the seq-hiwater guard at the driver; same event replayed twice is idempotent.
//   - knowledge_loop_transition_dedupes is NOT touched during reproject.
func KnowledgeLoopProjectorJob(
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	upsertEntryPort knowledge_loop_port.UpsertKnowledgeLoopEntryPort,
	upsertSessionPort knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort,
	upsertSurfacePort knowledge_loop_port.UpsertKnowledgeLoopSurfacePort,
) func(ctx context.Context) error {
	return KnowledgeLoopProjectorJobWithConfig(
		eventsPort,
		checkpointPort,
		updateCheckpointPort,
		upsertEntryPort,
		upsertSessionPort,
		upsertSurfacePort,
		KnowledgeLoopProjectorConfig{BatchSize: knowledgeLoopBatchSize},
	)
}

// KnowledgeLoopProjectorJobWithConfig is the config-bearing variant.
func KnowledgeLoopProjectorJobWithConfig(
	eventsPort knowledge_event_port.ListKnowledgeEventsPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort,
	upsertEntryPort knowledge_loop_port.UpsertKnowledgeLoopEntryPort,
	upsertSessionPort knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort,
	upsertSurfacePort knowledge_loop_port.UpsertKnowledgeLoopSurfacePort,
	cfg KnowledgeLoopProjectorConfig,
) func(ctx context.Context) error {
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = knowledgeLoopBatchSize
	}

	return func(ctx context.Context) error {
		log := logger.Logger

		lastSeq, err := checkpointPort.GetProjectionCheckpoint(ctx, knowledgeLoopProjectorName)
		if err != nil {
			return fmt.Errorf("knowledge_loop_projector: get checkpoint: %w", err)
		}

		events, err := eventsPort.ListKnowledgeEventsSince(ctx, lastSeq, batch)
		if err != nil {
			return fmt.Errorf("knowledge_loop_projector: list events: %w", err)
		}
		if len(events) == 0 {
			return nil
		}

		maxSeq := lastSeq
		projected := 0
		skipped := 0
		for i := range events {
			ev := events[i]
			res, err := projectLoopEvent(ctx, &ev, upsertEntryPort, upsertSessionPort, upsertSurfacePort)
			if err != nil {
				log.ErrorContext(ctx, "knowledge_loop_projector: skip event",
					"event_seq", ev.EventSeq,
					"event_type", ev.EventType,
					"err", err,
				)
				// continue: a bad individual event must not stall the whole projector
			}
			if res != nil && res.SkippedBySeqHiwater {
				skipped++
			}
			if res != nil && res.Applied {
				projected++
			}
			if ev.EventSeq > maxSeq {
				maxSeq = ev.EventSeq
			}
		}

		if err := updateCheckpointPort.UpdateProjectionCheckpoint(ctx, knowledgeLoopProjectorName, maxSeq); err != nil {
			return fmt.Errorf("knowledge_loop_projector: update checkpoint: %w", err)
		}

		log.InfoContext(ctx, "knowledge_loop_projector: batch complete",
			"projector", knowledgeLoopProjectorName,
			"from_seq", lastSeq,
			"to_seq", maxSeq,
			"events", len(events),
			"projected", projected,
			"skipped_by_guard", skipped,
		)
		return nil
	}
}

// projectLoopEvent turns a single knowledge_event into a projection effect.
// Returns a combined UpsertResult summary (applied if any sub-write applied).
func projectLoopEvent(
	ctx context.Context,
	ev *domain.KnowledgeEvent,
	upsertEntry knowledge_loop_port.UpsertKnowledgeLoopEntryPort,
	upsertSession knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort,
	_ knowledge_loop_port.UpsertKnowledgeLoopSurfacePort,
) (*knowledge_loop_port.UpsertResult, error) {
	if ev.UserID == nil {
		// System-level events (article creation etc.) are broadcast to the event log without
		// per-user fan-out at this layer. M3 projects user-addressed events only; fan-out to
		// follower users is a separate recall-projector concern.
		return nil, nil
	}
	lensModeID := defaultLoopLensModeID

	switch ev.EventType {
	case domain.EventSummaryVersionCreated, domain.EventHomeItemsSeen, domain.EventHomeItemAsked:
		entry, err := buildEntryFromEvent(ev, lensModeID, domain.LoopStageObserve, domain.SurfaceNow, domain.WhyKindSource)
		if err != nil {
			return nil, err
		}
		return upsertEntry.UpsertKnowledgeLoopEntry(ctx, entry)

	case domain.EventHomeItemOpened:
		entry, err := buildEntryFromEvent(ev, lensModeID, domain.LoopStageAct, domain.SurfaceContinue, domain.WhyKindChange)
		if err != nil {
			return nil, err
		}
		entry.DismissState = domain.DismissCompleted
		res, err := upsertEntry.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return nil, err
		}
		// Session state: user moved to Act.
		entryKey := entry.EntryKey
		state := &domain.KnowledgeLoopSessionState{
			UserID:                *ev.UserID,
			TenantID:              ev.TenantID,
			LensModeID:            lensModeID,
			CurrentStage:          domain.LoopStageAct,
			CurrentStageEnteredAt: ev.OccurredAt, // reproject-safe: from event, not NOW()
			LastActedEntryKey:     &entryKey,
			ProjectionSeqHiwater:  ev.EventSeq,
		}
		if _, sErr := upsertSession.UpsertKnowledgeLoopSessionState(ctx, state); sErr != nil {
			return res, fmt.Errorf("session upsert after opened: %w", sErr)
		}
		return res, nil

	case domain.EventHomeItemDismissed:
		entry, err := buildEntryFromEvent(ev, lensModeID, domain.LoopStageObserve, domain.SurfaceReview, domain.WhyKindSource)
		if err != nil {
			return nil, err
		}
		// Dismissed entries have no actionable CTA remaining.
		entry.DismissState = domain.DismissDismissed
		entry.DecisionOptions = nil
		return upsertEntry.UpsertKnowledgeLoopEntry(ctx, entry)

	case domain.EventHomeItemSuperseded:
		entry, err := buildEntryFromEvent(ev, lensModeID, domain.LoopStageObserve, domain.SurfaceChanged, domain.WhyKindChange)
		if err != nil {
			return nil, err
		}
		// Extract supersede target key from payload, if present.
		if target := extractStringField(ev.Payload, "new_entry_key", "superseded_by_entry_key"); target != "" {
			entry.SupersededByEntryKey = &target
		}
		return upsertEntry.UpsertKnowledgeLoopEntry(ctx, entry)

	case domain.EventKnowledgeLoopObserved,
		domain.EventKnowledgeLoopOriented,
		domain.EventKnowledgeLoopDecisionPresented,
		domain.EventKnowledgeLoopActed,
		domain.EventKnowledgeLoopReturned,
		domain.EventKnowledgeLoopDeferred,
		domain.EventKnowledgeLoopSessionReset,
		domain.EventKnowledgeLoopLensModeSwitched:
		return projectLoopTransitionEvent(ctx, ev, upsertSession)

	default:
		// Events we do not yet project (ArticleCreated at system-level, RecallSnoozed, etc.)
		return nil, nil
	}
}

// projectLoopTransitionEvent consumes a /loop-originated transition event and
// updates knowledge_loop_session_state. It never upserts an entry row —
// entries come from article-side events (/feeds HomeItem* or SummaryVersionCreated).
//
// ADR-000831 §3.8 (single-emission rule) pins this split: /loop transitions write
// session-state, /feeds actions write entries via HomeItem*.
//
// Reproject-safety:
//   - CurrentStageEnteredAt derives from event.OccurredAt (never wall-clock).
//   - ProjectionSeqHiwater = event.EventSeq; sovereign-side UPSERT guards with
//     `WHERE session.projection_seq_hiwater <= EXCLUDED.projection_seq_hiwater`
//     and COALESCE on last_*_entry_key, so older events cannot overwrite newer
//     pointers on replay (merge-safe upsert invariant, ADR finding F1).
func projectLoopTransitionEvent(
	ctx context.Context,
	ev *domain.KnowledgeEvent,
	upsertSession knowledge_loop_port.UpsertKnowledgeLoopSessionStatePort,
) (*knowledge_loop_port.UpsertResult, error) {
	payload := parseLoopTransitionPayload(ev.Payload)
	lensModeID := payload.LensModeID
	if lensModeID == "" {
		lensModeID = defaultLoopLensModeID
	}

	state := &domain.KnowledgeLoopSessionState{
		UserID:                *ev.UserID,
		TenantID:              ev.TenantID,
		LensModeID:            lensModeID,
		CurrentStage:          mapStageNameOrFallback(payload.ToStage, stageForLoopEvent(ev.EventType)),
		CurrentStageEnteredAt: ev.OccurredAt,
		ProjectionSeqHiwater:  ev.EventSeq,
	}

	// Only the field matching this event's semantic target is populated; the
	// sovereign UPSERT applies COALESCE to preserve older last_*_entry_key values.
	entryKey := payload.EntryKey
	if entryKey != "" {
		switch ev.EventType {
		case domain.EventKnowledgeLoopObserved:
			state.LastObservedEntryKey = &entryKey
		case domain.EventKnowledgeLoopOriented:
			state.LastOrientedEntryKey = &entryKey
		case domain.EventKnowledgeLoopDecisionPresented:
			state.LastDecidedEntryKey = &entryKey
		case domain.EventKnowledgeLoopActed:
			state.LastActedEntryKey = &entryKey
		case domain.EventKnowledgeLoopReturned:
			state.LastReturnedEntryKey = &entryKey
		case domain.EventKnowledgeLoopDeferred:
			state.LastDeferredEntryKey = &entryKey
		}
	}

	return upsertSession.UpsertKnowledgeLoopSessionState(ctx, state)
}

type loopTransitionPayload struct {
	EntryKey   string
	LensModeID string
	FromStage  string
	ToStage    string
	Trigger    string
}

// parseLoopTransitionPayload decodes the structured payload written by
// TransitionKnowledgeLoopUsecase.buildTransitionEvent. Missing fields yield zero
// values; the caller tolerates them so a schema drift between write and read
// paths does not panic the projector.
func parseLoopTransitionPayload(raw json.RawMessage) loopTransitionPayload {
	out := loopTransitionPayload{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	out.EntryKey = pickString(m, "entry_key")
	out.LensModeID = pickString(m, "lens_mode_id")
	out.FromStage = pickString(m, "from_stage")
	out.ToStage = pickString(m, "to_stage")
	out.Trigger = pickString(m, "trigger")
	return out
}

// mapStageNameOrFallback resolves a proto enum string (e.g. "LOOP_STAGE_ORIENT")
// to the domain.LoopStage, falling back to the fallback value if the name is
// missing or unrecognized.
func mapStageNameOrFallback(name string, fallback domain.LoopStage) domain.LoopStage {
	switch name {
	case "LOOP_STAGE_OBSERVE":
		return domain.LoopStageObserve
	case "LOOP_STAGE_ORIENT":
		return domain.LoopStageOrient
	case "LOOP_STAGE_DECIDE":
		return domain.LoopStageDecide
	case "LOOP_STAGE_ACT":
		return domain.LoopStageAct
	}
	return fallback
}

// stageForLoopEvent returns the canonical stage associated with a Loop
// transition event type. Used as a fallback when the payload lacks to_stage.
func stageForLoopEvent(eventType string) domain.LoopStage {
	switch eventType {
	case domain.EventKnowledgeLoopObserved,
		domain.EventKnowledgeLoopReturned,
		domain.EventKnowledgeLoopSessionReset:
		return domain.LoopStageObserve
	case domain.EventKnowledgeLoopOriented,
		domain.EventKnowledgeLoopDeferred:
		return domain.LoopStageOrient
	case domain.EventKnowledgeLoopDecisionPresented:
		return domain.LoopStageDecide
	case domain.EventKnowledgeLoopActed:
		return domain.LoopStageAct
	}
	return domain.LoopStageObserve
}

// buildEntryFromEvent materializes a KnowledgeLoopEntry from an event, filling reproject-safe
// timestamps from event.occurred_at and a minimal WhyPayload. Caller can override fields.
func buildEntryFromEvent(
	ev *domain.KnowledgeEvent,
	lensModeID string,
	proposedStage domain.LoopStage,
	surfaceBucket domain.SurfaceBucket,
	whyKind domain.WhyKind,
) (*domain.KnowledgeLoopEntry, error) {
	if ev.UserID == nil {
		return nil, fmt.Errorf("event has no user_id; cannot project to Knowledge Loop entry")
	}
	entryKey, err := deriveEntryKey(ev)
	if err != nil {
		return nil, err
	}
	sourceItemKey := entryKey

	// Artifact version ref: fill from event payload (summary_version_id, tag_set_version_id,
	// lens_version_id). At least one is required by DB CHECK; projector falls back to a
	// synthetic lens version when the event has none, so the entry remains insertable during
	// reproject of historical events.
	art := extractArtifactVersionRef(ev.Payload)
	if art.SummaryVersionID == nil && art.TagSetVersionID == nil && art.LensVersionID == nil {
		fallback := "lens:" + lensModeID
		art.LensVersionID = &fallback
	}

	// Why enrichment (ADR-000840 / WhyMappingVersion=2). The enricher is a pure
	// function over the event payload, so reproject replays produce identical
	// why_text and evidence_refs without reading latest projection state.
	enriched := knowledge_loop_usecase.EnrichWhyFromEvent(ev)
	// The enricher is the authoritative classifier — it always returns a valid
	// WhyKind. The caller-supplied whyKind is kept as a default for unknown
	// events only, since the enricher falls back to Source for those too.
	finalKind := enriched.Kind
	if finalKind == "" {
		finalKind = whyKind
	}
	refIDs := make([]string, 0, len(enriched.EvidenceRefs))
	for _, r := range enriched.EvidenceRefs {
		refIDs = append(refIDs, r.RefID)
	}

	return &domain.KnowledgeLoopEntry{
		UserID:               *ev.UserID,
		TenantID:             ev.TenantID,
		LensModeID:           lensModeID,
		EntryKey:             entryKey,
		SourceItemKey:        sourceItemKey,
		ProposedStage:        proposedStage,
		SurfaceBucket:        surfaceBucket,
		ProjectionSeqHiwater: ev.EventSeq,
		SourceEventSeq:       ev.EventSeq,
		FreshnessAt:          ev.OccurredAt, // reproject-safe
		ArtifactVersionRef:   art,
		WhyKind:              finalKind,
		WhyText:              enriched.Text,
		WhyEvidenceRefs:      enriched.EvidenceRefs,
		WhyEvidenceRefIDs:    refIDs,
		DecisionOptions:      seedDecisionOptions(whyKind, proposedStage),
		DismissState:         domain.DismissActive,
		RenderDepthHint:      pickRenderDepth(surfaceBucket),
		LoopPriority:         pickLoopPriority(surfaceBucket),
	}, nil
}

// seedDecisionOptions materializes the CTA options the UI can offer for a newly
// projected entry. The mapping is intentionally terse — a source-driven entry
// (new article, fresh summary) is almost always actionable through the same
// four paths; other why kinds keep empty options until the projector learns a
// specific Decide-stage strategy for them (ADR-000831 / plan D1).
//
// Reproject-safe: the seed is derived from the (static) why_kind and
// proposed_stage alone, never from latest projection state or wall-clock.
func seedDecisionOptions(whyKind domain.WhyKind, stage domain.LoopStage) []byte {
	if whyKind != domain.WhyKindSource {
		return nil
	}
	// Observe / Orient entries can offer all four; Decide/Act entries are
	// intentionally left to the handler to narrow (Act has already happened).
	if stage != domain.LoopStageObserve && stage != domain.LoopStageOrient {
		return nil
	}
	// Ordered so the UI can treat the first as primary CTA.
	type opt struct {
		ActionID string `json:"action_id"`
		Intent   string `json:"intent"`
		Label    string `json:"label,omitempty"`
	}
	seed := []opt{
		{ActionID: "open", Intent: "open"},
		{ActionID: "ask", Intent: "ask"},
		{ActionID: "save", Intent: "save"},
		{ActionID: "dismiss", Intent: "snooze"},
	}
	b, err := json.Marshal(seed)
	if err != nil {
		return nil
	}
	return b
}

// deriveEntryKey picks a stable, format-valid entry key from the event.
// Priority: explicit entry_key → aggregate_id with aggregate_type prefix → fallback to event_id.
// Result must match ^[A-Za-z0-9_:-]{1,128}$ so it passes the DB CHECK.
func deriveEntryKey(ev *domain.KnowledgeEvent) (string, error) {
	if key := extractStringField(ev.Payload, "entry_key", "item_key"); key != "" {
		if isSafeKey(key) {
			return key, nil
		}
	}
	if ev.AggregateType != "" && ev.AggregateID != "" {
		candidate := fmt.Sprintf("%s:%s", ev.AggregateType, sanitizeKeySegment(ev.AggregateID))
		if isSafeKey(candidate) {
			return candidate, nil
		}
	}
	// Fallback to a UUID-based synthetic key so reproject never stalls on missing payload.
	return "event:" + ev.EventID.String(), nil
}

func pickRenderDepth(bucket domain.SurfaceBucket) domain.RenderDepthHint {
	switch bucket {
	case domain.SurfaceNow:
		return domain.RenderDepthStrong
	case domain.SurfaceChanged:
		return domain.RenderDepthLight
	case domain.SurfaceContinue:
		return domain.RenderDepthLight
	case domain.SurfaceReview:
		return domain.RenderDepthFlat
	default:
		return domain.RenderDepthFlat
	}
}

func pickLoopPriority(bucket domain.SurfaceBucket) domain.LoopPriority {
	switch bucket {
	case domain.SurfaceNow:
		return domain.LoopPriorityCritical
	case domain.SurfaceContinue:
		return domain.LoopPriorityContinuing
	case domain.SurfaceChanged:
		return domain.LoopPriorityConfirm
	case domain.SurfaceReview:
		return domain.LoopPriorityReference
	default:
		return domain.LoopPriorityReference
	}
}

// extractStringField scans a JSON payload for the first non-empty string at one of the given keys.
// Returns "" if none found or payload is not an object.
func extractStringField(payload json.RawMessage, keys ...string) string {
	if len(payload) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// extractArtifactVersionRef pulls well-known version IDs from an event payload.
func extractArtifactVersionRef(payload json.RawMessage) domain.ArtifactVersionRef {
	ref := domain.ArtifactVersionRef{}
	if len(payload) == 0 {
		return ref
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ref
	}
	if v := pickString(m, "summary_version_id"); v != "" {
		ref.SummaryVersionID = &v
	}
	if v := pickString(m, "tag_set_version_id"); v != "" {
		ref.TagSetVersionID = &v
	}
	if v := pickString(m, "lens_version_id"); v != "" {
		ref.LensVersionID = &v
	}
	return ref
}

func pickString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// isSafeKey validates that key matches the canonical ^[A-Za-z0-9_:-]{1,128}$ format,
// so it can be written directly to DB-side CHECK-guarded columns.
func isSafeKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '_' || c == ':' || c == '-':
		default:
			return false
		}
	}
	return true
}

// sanitizeKeySegment transforms an arbitrary aggregate_id (often a URL or UUID)
// into something that passes isSafeKey when combined with a type prefix.
func sanitizeKeySegment(raw string) string {
	if _, err := uuid.Parse(raw); err == nil {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == '_' || c == ':' || c == '-':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 128-16 {
			break
		}
	}
	return b.String()
}
