package job

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_loop_port"
	"alt/port/knowledge_projection_port"
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
		entry.DismissState = domain.DismissDismissed
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

	default:
		// Events we do not yet project (ArticleCreated at system-level, RecallSnoozed, etc.)
		return nil, nil
	}
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
		WhyKind:              whyKind,
		WhyText:              shortEventWhy(ev),
		DismissState:         domain.DismissActive,
		RenderDepthHint:      pickRenderDepth(surfaceBucket),
		LoopPriority:         pickLoopPriority(surfaceBucket),
	}, nil
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

// shortEventWhy builds a terse plain-text rationale under 512 chars.
// It MUST NOT contain markdown or HTML (per canonical contract).
func shortEventWhy(ev *domain.KnowledgeEvent) string {
	switch ev.EventType {
	case domain.EventHomeItemOpened:
		return "You opened this item."
	case domain.EventHomeItemDismissed:
		return "You dismissed this item."
	case domain.EventHomeItemSuperseded:
		return "This item was superseded by a newer version."
	case domain.EventSummaryVersionCreated:
		return "A new summary version was produced."
	case domain.EventHomeItemsSeen:
		return "Surfaced in your home feed."
	case domain.EventHomeItemAsked:
		return "You asked about this item."
	default:
		return "Surfaced from a recent event."
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
