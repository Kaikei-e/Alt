// Package surface_planner_cron emits KnowledgeLoopSurfacePlanRecomputed
// system events when upstream signals (AugurConversationLinked is the v1
// scope; RecapTopicSnapshotted is wired-ready but skipped until tag fan-out
// lands) suggest existing Knowledge Loop entries should re-evaluate their
// surface placement.
//
// The producer is the missing half of the projector branch added in
// ADR-000873: without it, the system event has no source. The projector
// branch is patch-only and seq-hiwater-guarded, so over-emission degrades
// gracefully to no-op patches.
//
// Reproject-safety: the cron runs on wall-clock time but the event payloads
// it emits derive their occurred_at strictly from the triggering signal's
// occurred_at. Replaying the event log produces deterministic projections
// regardless of when the cron fired.
package surface_planner_cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/knowledge_loop_projector"
)

// PlannerName is the row key used in knowledge_projection_checkpoints. It is
// distinct from the knowledge_loop_projector's row so the producer's reading
// frontier advances independently of the projector's.
const PlannerName = "surface_planner_v2"

const defaultLensModeID = "default"

// signalEventTypes is the v1 allowlist of upstream events that may warrant a
// SurfacePlanRecomputed emit. The set is intentionally narrow:
//
//   - AugurConversationLinked carries entry_key in its payload, so we can
//     name the affected entry directly in entry_inputs[].
//
// RecapTopicSnapshotted lacks an entry_key and would require a tag fan-out
// against entry sources; that is a separate work item.
//
// Events that are already projected as entry-mutating in the projector switch
// (HomeItemOpened, SummaryVersionCreated, SummarySuperseded) are intentionally
// excluded so the producer does not double-fire on the same logical change.
var signalEventTypes = map[string]struct{}{
	knowledge_loop_projector.EventAugurConversationLinked: {},
}

// Repository captures the narrow surface the cron needs from sovereign_db.
// Keeping the interface tight makes the unit test fake easy to maintain.
type Repository interface {
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	AppendKnowledgeEvent(ctx context.Context, event sovereign_db.KnowledgeEvent) (int64, error)
}

// Config tunes the cron loop. Zero values fall back to defaults.
type Config struct {
	BatchSize         int
	MaxBatchesPerTick int
}

// Cron is the surface planner v2 producer. It mirrors the structure of
// knowledge_loop_projector.Projector so operators see the same shape.
type Cron struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
}

// New constructs a cron with sensible defaults.
func New(repo Repository, logger *slog.Logger, cfg Config) *Cron {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 256
	}
	if cfg.MaxBatchesPerTick <= 0 {
		cfg.MaxBatchesPerTick = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Cron{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch reads new signal events from the checkpoint forward, groups them
// by (tenant_id, user_id) and emits one KnowledgeLoopSurfacePlanRecomputed
// per group with all affected entry_keys carried in entry_inputs[].
func (c *Cron) RunBatch(ctx context.Context) error {
	lastSeq, err := c.repo.GetProjectionCheckpoint(ctx, PlannerName)
	if err != nil {
		return fmt.Errorf("surface_planner_cron: get checkpoint: %w", err)
	}

	currentSeq := lastSeq
	totalEvents := 0
	totalEmitted := 0
	batches := 0

	for batches < c.cfg.MaxBatchesPerTick {
		events, err := c.repo.ListKnowledgeEventsSince(ctx, currentSeq, c.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("surface_planner_cron: list events: %w", err)
		}
		if len(events) == 0 {
			break
		}
		batches++
		totalEvents += len(events)

		// Group signal events by (tenant_id, user_id). Inside a group, dedupe by
		// entry_key — a user can have several Augur links touching the same
		// entry within one tick and we want a single entry_input per entry.
		type groupKey struct {
			tenant uuid.UUID
			user   uuid.UUID
		}
		type group struct {
			entries     map[string]struct{}
			batchMaxSeq int64
			anchor      sovereign_db.KnowledgeEvent
		}
		groups := map[groupKey]*group{}
		maxSeq := currentSeq

		for i := range events {
			ev := events[i]
			if ev.EventSeq > maxSeq {
				maxSeq = ev.EventSeq
			}
			if _, ok := signalEventTypes[ev.EventType]; !ok {
				continue
			}
			if ev.UserID == nil {
				continue
			}
			entryKey := readPayloadEntryKey(ev.Payload)
			if entryKey == "" {
				continue
			}
			key := groupKey{tenant: ev.TenantID, user: *ev.UserID}
			g, ok := groups[key]
			if !ok {
				g = &group{entries: map[string]struct{}{}, anchor: ev, batchMaxSeq: ev.EventSeq}
				groups[key] = g
			}
			g.entries[entryKey] = struct{}{}
			if ev.EventSeq > g.batchMaxSeq {
				g.batchMaxSeq = ev.EventSeq
				// keep the latest signal as the group anchor so occurred_at
				// reflects the most recent context change.
				g.anchor = ev
			}
		}

		emitted := 0
		for key, g := range groups {
			if len(g.entries) == 0 {
				continue
			}
			entryKeys := make([]string, 0, len(g.entries))
			for k := range g.entries {
				entryKeys = append(entryKeys, k)
			}
			sort.Strings(entryKeys)

			ev, err := buildSurfacePlanEvent(key.tenant, key.user, g.anchor, g.batchMaxSeq, entryKeys)
			if err != nil {
				c.logger.ErrorContext(ctx, "surface_planner_cron: build event failed",
					slog.String("err", err.Error()))
				continue
			}
			if _, err := c.repo.AppendKnowledgeEvent(ctx, ev); err != nil {
				c.logger.ErrorContext(ctx, "surface_planner_cron: append failed",
					slog.String("err", err.Error()))
				continue
			}
			emitted++
		}

		if err := c.repo.UpdateProjectionCheckpoint(ctx, PlannerName, maxSeq); err != nil {
			return fmt.Errorf("surface_planner_cron: update checkpoint: %w", err)
		}
		currentSeq = maxSeq
		totalEmitted += emitted

		if len(events) < c.cfg.BatchSize {
			break
		}
	}

	c.logger.InfoContext(ctx, "surface_planner.batch_complete",
		slog.String("planner", PlannerName),
		slog.Int64("from_seq", lastSeq),
		slog.Int64("to_seq", currentSeq),
		slog.Int("batches", batches),
		slog.Int("events_seen", totalEvents),
		slog.Int("emitted", totalEmitted),
	)
	return nil
}

// buildSurfacePlanEvent assembles the SurfacePlanRecomputed event payload.
// The dedupe_key is keyed on (user, lens_mode, batch_max_seq) so reruns of
// the same batch are no-ops at the AppendKnowledgeEvent layer.
func buildSurfacePlanEvent(
	tenantID, userID uuid.UUID,
	anchor sovereign_db.KnowledgeEvent,
	batchMaxSeq int64,
	entryKeys []string,
) (sovereign_db.KnowledgeEvent, error) {
	entryInputs := make([]map[string]any, 0, len(entryKeys))
	for _, k := range entryKeys {
		entryInputs = append(entryInputs, map[string]any{
			"entry_key":                   k,
			"event_type":                  knowledge_loop_projector.EventKnowledgeLoopSurfacePlanRecomputed,
			"has_augur_link":              true,
			"freshness_at":                anchor.OccurredAt.UTC().Format(time.RFC3339Nano),
			"source_observed_at":          anchor.OccurredAt.UTC().Format(time.RFC3339Nano),
			"question_continuation_score": uint32(1),
		})
	}
	body := map[string]any{
		"lens_mode_id":    defaultLensModeID,
		"planner_version": "SURFACE_PLANNER_VERSION_V2",
		"entry_inputs":    entryInputs,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}

	dedupeKey := fmt.Sprintf("%s:%s:%s:%d",
		knowledge_loop_projector.EventKnowledgeLoopSurfacePlanRecomputed,
		userID.String(), defaultLensModeID, batchMaxSeq,
	)

	uid := userID
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    anchor.OccurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     "system",
		ActorID:       "surface_planner_v2",
		EventType:     knowledge_loop_projector.EventKnowledgeLoopSurfacePlanRecomputed,
		AggregateType: knowledge_loop_projector.AggregateLoopSession,
		AggregateID:   defaultLensModeID,
		DedupeKey:     dedupeKey,
		Payload:       payload,
	}, nil
}

// readPayloadEntryKey pulls entry_key out of a JSON payload using the same
// alternative key set the projector accepts (entry_key|item_key).
func readPayloadEntryKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, k := range []string{"entry_key", "item_key", "entryKey", "itemKey"} {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
