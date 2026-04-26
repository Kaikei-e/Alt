package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

const (
	// projectorName is the row key used in projection_checkpoints to track how
	// far this projector has consumed the knowledge_events log.
	projectorName     = "knowledge-loop-projector"
	defaultBatchSize  = 100
	defaultLensModeID = "default"
)

// Repository is the subset of sovereign_db.Repository that the projector needs.
// Defining it here keeps the package unit-testable with an in-memory fake and
// documents the projector's only allowed coupling to durable state.
type Repository interface {
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	UpsertKnowledgeLoopEntry(ctx context.Context, e *sovereignv1.KnowledgeLoopEntry) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopSessionState(ctx context.Context, s *sovereignv1.KnowledgeLoopSessionState) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// PatchKnowledgeLoopEntryWhy updates only the why_* columns of an existing
	// entry row; dismiss_state, freshness_at, surface_bucket and other fields
	// are preserved. Used by the SummaryNarrativeBackfilled projector branch
	// (ADR-000846) to repair historic entries whose original
	// SummaryVersionCreated event lacked article_title in payload.
	PatchKnowledgeLoopEntryWhy(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, why *sovereignv1.KnowledgeLoopWhyPayload) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// PatchKnowledgeLoopEntryDismissState updates only the dismiss_state column of
	// an existing entry row; why_text, freshness_at, surface_bucket and any other
	// field are preserved. Used by the KnowledgeLoopDeferred projector branch to
	// flip an entry to `deferred` (canonical contract §8.2 passive dismiss) without
	// clobbering the row's other state. The driver MUST enforce a
	// `projection_seq_hiwater <= eventSeq` guard so replays are idempotent and
	// out-of-order events are no-ops.
	PatchKnowledgeLoopEntryDismissState(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, dismissState sovereignv1.DismissState) (*sovereign_db.KnowledgeLoopUpsertResult, error)
}

// Config tunes the projector loop. Zero values fall back to defaults.
type Config struct {
	BatchSize int
}

// Projector runs the Knowledge Loop projection job over knowledge_events.
//
// Reproject-safety invariants (see docs/plan/knowledge-loop-canonical-contract.md):
//   - Reads only event payloads. Never reads latest projection state.
//   - freshness_at and current_stage_entered_at come from event.occurred_at, never wall-clock.
//   - UPSERTs enforce the seq-hiwater guard at the driver; same event replayed twice is idempotent.
//   - knowledge_loop_transition_dedupes is NOT touched during reproject.
type Projector struct {
	repo          Repository
	logger        *slog.Logger
	cfg           Config
	scoreResolver SurfaceScoreResolver
}

// NewProjector wires a repository + logger into a projector. Use RunBatch on a
// timer (the service's main loop owns the cadence) — Projector does not own a
// goroutine itself.
//
// The default score resolver is NullSurfaceScoreResolver: bucket placement
// stays on the v1 mapping until a real resolver is plugged via
// WithScoreResolver. This keeps Wave 2/3 changes additive — the live
// projector still writes the same buckets it always did, but the v2 hook
// + metrics are reachable from tests.
func NewProjector(repo Repository, logger *slog.Logger, cfg Config) *Projector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	return &Projector{
		repo:          repo,
		logger:        logger,
		cfg:           cfg,
		scoreResolver: NullSurfaceScoreResolver{},
	}
}

// WithScoreResolver swaps the SurfaceScoreResolver used to compute v2
// bucket inputs. Wave 4 will plug a cross-service resolver here that
// queries tag_set_versions / recap_topic_snapshots / augur_conversations
// with mandatory user_id binding.
func (p *Projector) WithScoreResolver(r SurfaceScoreResolver) *Projector {
	if r != nil {
		p.scoreResolver = r
	}
	return p
}

// RunBatch consumes one batch of events from the checkpoint forward. The caller
// schedules invocations (typically every few seconds). Errors are logged and
// the checkpoint advances only across the events successfully read; bad
// individual events are skipped without stalling the whole projector.
func (p *Projector) RunBatch(ctx context.Context) error {
	lastSeq, err := p.repo.GetProjectionCheckpoint(ctx, projectorName)
	if err != nil {
		return fmt.Errorf("knowledge_loop_projector: get checkpoint: %w", err)
	}

	events, err := p.repo.ListKnowledgeEventsSince(ctx, lastSeq, p.cfg.BatchSize)
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
		res, err := p.projectEvent(ctx, &ev)
		if err != nil {
			p.logger.ErrorContext(ctx, "knowledge_loop_projector: skip event",
				slog.Int64("event_seq", ev.EventSeq),
				slog.String("event_type", ev.EventType),
				slog.String("err", err.Error()),
			)
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

	if err := p.repo.UpdateProjectionCheckpoint(ctx, projectorName, maxSeq); err != nil {
		return fmt.Errorf("knowledge_loop_projector: update checkpoint: %w", err)
	}

	p.logger.InfoContext(ctx, "knowledge_loop_projector: batch complete",
		slog.String("projector", projectorName),
		slog.Int64("from_seq", lastSeq),
		slog.Int64("to_seq", maxSeq),
		slog.Int("events", len(events)),
		slog.Int("projected", projected),
		slog.Int("skipped_by_guard", skipped),
	)
	return nil
}

// projectEvent dispatches a single event to the right projection write path.
// Returns (result, err); a non-nil err is logged and the projector continues.
func (p *Projector) projectEvent(ctx context.Context, ev *sovereign_db.KnowledgeEvent) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	if ev.UserID == nil {
		// System-level events (e.g. ArticleCreated) lack per-user fan-out at
		// this layer and are intentionally a no-op here.
		return nil, nil
	}
	lensModeID := defaultLensModeID

	switch ev.EventType {
	case EventSummaryVersionCreated, EventHomeItemsSeen, EventHomeItemAsked:
		bucket, inputs := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs)
		if err != nil {
			return nil, err
		}
		return p.repo.UpsertKnowledgeLoopEntry(ctx, entry)

	case EventSummaryNarrativeBackfilled:
		// ADR-000846: discovered event repairs historic entries' why_text via
		// the patch path. The full UPSERT is intentionally avoided — it would
		// overwrite dismiss_state and other entry-level fields that the user
		// (or earlier events) may have transitioned. The patch SQL touches
		// only the why_* columns; seq-hiwater guard ensures replay safety.
		return p.projectSummaryNarrativeBackfilled(ctx, ev, lensModeID)

	case EventHomeItemOpened:
		bucket, inputs := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_ACT,
			bucket, inputs)
		if err != nil {
			return nil, err
		}
		entry.DismissState = sovereignv1.DismissState_DISMISS_STATE_COMPLETED
		res, err := p.repo.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return nil, err
		}
		entryKey := entry.EntryKey
		state := &sovereignv1.KnowledgeLoopSessionState{
			UserId:                ev.UserID.String(),
			TenantId:              ev.TenantID.String(),
			LensModeId:            lensModeID,
			CurrentStage:          sovereignv1.LoopStage_LOOP_STAGE_ACT,
			CurrentStageEnteredAt: timestamppb.New(ev.OccurredAt),
			LastActedEntryKey:     &entryKey,
			ProjectionSeqHiwater:  ev.EventSeq,
		}
		if _, sErr := p.repo.UpsertKnowledgeLoopSessionState(ctx, state); sErr != nil {
			return res, fmt.Errorf("session upsert after opened: %w", sErr)
		}
		return res, nil

	case EventHomeItemDismissed:
		bucket, inputs := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs)
		if err != nil {
			return nil, err
		}
		entry.DismissState = sovereignv1.DismissState_DISMISS_STATE_DISMISSED
		entry.DecisionOptions = nil
		return p.repo.UpsertKnowledgeLoopEntry(ctx, entry)

	case EventHomeItemSuperseded, EventSummarySuperseded:
		bucket, inputs := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs)
		if err != nil {
			return nil, err
		}
		if target := extractStringField(ev.Payload, "new_entry_key", "superseded_by_entry_key"); target != "" {
			entry.SupersededByEntryKey = &target
		}
		// Populate change_summary JSONB. When the event payload carries old
		// + new summary excerpts (or tag arrays) the redline diff fields are
		// filled by computeChangeDiff; otherwise the legacy summary +
		// changed_fields shape is preserved. Fully reproject-safe — inputs
		// are read from event payload only.
		csPayload := parseChangeSummaryPayload(ev.Payload)
		if cs := buildChangeSummaryJSON(csPayload); cs != nil {
			entry.ChangeSummary = cs
			redlineCapable := csPayload.canRedline()
			slog.DebugContext(ctx, "knowledge_loop_projector: change_summary written",
				"event_type", ev.EventType,
				"event_seq", ev.EventSeq,
				"entry_key", entry.EntryKey,
				"redline_capable", redlineCapable,
				"change_summary_bytes", len(cs),
			)
			observeChangeSummaryWritten(redlineCapable)
		}
		return p.repo.UpsertKnowledgeLoopEntry(ctx, entry)

	case EventKnowledgeLoopObserved,
		EventKnowledgeLoopOriented,
		EventKnowledgeLoopDecisionPresented,
		EventKnowledgeLoopActed,
		EventKnowledgeLoopReturned,
		EventKnowledgeLoopDeferred,
		EventKnowledgeLoopSessionReset,
		EventKnowledgeLoopLensModeSwitched:
		return p.projectTransition(ctx, ev)

	default:
		return nil, nil
	}
}

// projectSummaryNarrativeBackfilled handles the discovered event emitted by
// alt-backend's summary-narrative-backfill job (ADR-000846). It calls the
// patch-only-why repo path so the existing entry's why_text is repaired
// without disturbing dismiss_state, freshness_at, or any other field. The
// projector is reproject-safe: enrichment uses event payload only.
func (p *Projector) projectSummaryNarrativeBackfilled(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
	lensModeID string,
) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	if ev.UserID == nil {
		return nil, nil
	}
	entryKey, err := deriveEntryKey(ev)
	if err != nil {
		return nil, err
	}
	why := EnrichWhyFromEvent(ev)
	return p.repo.PatchKnowledgeLoopEntryWhy(
		ctx,
		ev.UserID.String(),
		ev.TenantID.String(),
		lensModeID,
		entryKey,
		ev.EventSeq,
		why,
	)
}

// projectTransition handles /loop-originated transition events. ADR-000831 §3.7
// (single-emission rule) pins this split: /loop transitions write session state,
// /feeds actions write entries.
//
// The Deferred branch additionally patches the entry's dismiss_state to
// DEFERRED via PatchKnowledgeLoopEntryDismissState (canonical contract §8.2).
// The patch is a single-column UPDATE guarded by projection_seq_hiwater so
// reproject and out-of-order delivery remain idempotent — no other entry
// fields are touched, preserving why_text / freshness_at / surface_bucket.
func (p *Projector) projectTransition(ctx context.Context, ev *sovereign_db.KnowledgeEvent) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	payload := parseLoopTransitionPayload(ev.Payload)
	lensModeID := payload.LensModeID
	if lensModeID == "" {
		lensModeID = defaultLensModeID
	}

	state := &sovereignv1.KnowledgeLoopSessionState{
		UserId:                ev.UserID.String(),
		TenantId:              ev.TenantID.String(),
		LensModeId:            lensModeID,
		CurrentStage:          mapStageNameOrFallback(payload.ToStage, stageForLoopEvent(ev.EventType)),
		CurrentStageEnteredAt: timestamppb.New(ev.OccurredAt),
		ProjectionSeqHiwater:  ev.EventSeq,
	}

	entryKey := payload.EntryKey
	if entryKey != "" {
		switch ev.EventType {
		case EventKnowledgeLoopObserved:
			state.LastObservedEntryKey = &entryKey
		case EventKnowledgeLoopOriented:
			state.LastOrientedEntryKey = &entryKey
		case EventKnowledgeLoopDecisionPresented:
			state.LastDecidedEntryKey = &entryKey
		case EventKnowledgeLoopActed:
			state.LastActedEntryKey = &entryKey
		case EventKnowledgeLoopReturned:
			state.LastReturnedEntryKey = &entryKey
		case EventKnowledgeLoopDeferred:
			state.LastDeferredEntryKey = &entryKey
		}
	}

	sessionRes, sessionErr := p.repo.UpsertKnowledgeLoopSessionState(ctx, state)
	if sessionErr != nil {
		return sessionRes, sessionErr
	}

	// Deferred uniquely flips the entry's dismiss_state. All other transitions
	// leave the entry row untouched — they only update session state above.
	if ev.EventType == EventKnowledgeLoopDeferred && entryKey != "" {
		if _, patchErr := p.repo.PatchKnowledgeLoopEntryDismissState(
			ctx,
			ev.UserID.String(),
			ev.TenantID.String(),
			lensModeID,
			entryKey,
			ev.EventSeq,
			sovereignv1.DismissState_DISMISS_STATE_DEFERRED,
		); patchErr != nil {
			return sessionRes, fmt.Errorf("patch dismiss_state on Deferred: %w", patchErr)
		}
	}

	return sessionRes, nil
}

// buildEntryFromEvent materializes a KnowledgeLoopEntry from an event with
// reproject-safe timestamps and a Why payload from the enricher. When
// `inputs` carries a non-zero Surface Planner v2 signal, the enricher
// output is re-stamped with the v3 narrative kind (topic_affinity_why /
// tag_trending_why / unfinished_continue_why) before the entry is
// returned. Callers that don't need v2 inputs may pass the empty
// SurfaceScoreInputs{} — the override is a no-op then.
func (p *Projector) buildEntryFromEvent(
	ev *sovereign_db.KnowledgeEvent,
	lensModeID string,
	proposedStage sovereignv1.LoopStage,
	surfaceBucket sovereignv1.SurfaceBucket,
	inputs SurfaceScoreInputs,
) (*sovereignv1.KnowledgeLoopEntry, error) {
	if ev.UserID == nil {
		return nil, fmt.Errorf("event has no user_id; cannot project to Knowledge Loop entry")
	}
	entryKey, err := deriveEntryKey(ev)
	if err != nil {
		return nil, err
	}
	sourceItemKey := entryKey

	art := extractArtifactVersionRef(ev.Payload)
	if art.SummaryVersionId == nil && art.TagSetVersionId == nil && art.LensVersionId == nil {
		fallback := "lens:" + lensModeID
		art.LensVersionId = &fallback
	}

	why := EnrichWhyFromEvent(ev)
	why = OverrideWhyFromSurfaceInputs(ev, why, inputs)

	return &sovereignv1.KnowledgeLoopEntry{
		UserId:               ev.UserID.String(),
		TenantId:             ev.TenantID.String(),
		LensModeId:           lensModeID,
		EntryKey:             entryKey,
		SourceItemKey:        sourceItemKey,
		ProposedStage:        proposedStage,
		SurfaceBucket:        surfaceBucket,
		ProjectionSeqHiwater: ev.EventSeq,
		SourceEventSeq:       ev.EventSeq,
		FreshnessAt:          timestamppb.New(ev.OccurredAt),
		ArtifactVersionRef:   art,
		WhyPrimary:           why,
		DecisionOptions:      seedDecisionOptions(proposedStage),
		DismissState:         sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
		RenderDepthHint:      pickRenderDepth(surfaceBucket),
		LoopPriority:         pickLoopPriority(surfaceBucket),
	}, nil
}

// seedDecisionOptions materializes the CTA options the UI can offer for a
// newly projected entry. Each option is paired with a stage transition the user
// is allowed to take from the entry's proposed_stage per canonical contract §7.
//
// Reproject-safe: depends only on (static) proposed_stage, never on latest
// projection state or wall-clock.
func seedDecisionOptions(stage sovereignv1.LoopStage) []byte {
	type opt struct {
		ActionID string `json:"action_id"`
		Intent   string `json:"intent"`
		Label    string `json:"label,omitempty"`
	}
	var seed []opt
	switch stage {
	case sovereignv1.LoopStage_LOOP_STAGE_OBSERVE:
		seed = []opt{
			{ActionID: "revisit", Intent: "revisit"},
			{ActionID: "ask", Intent: "ask"},
			{ActionID: "snooze", Intent: "snooze"},
		}
	case sovereignv1.LoopStage_LOOP_STAGE_ORIENT:
		seed = []opt{
			{ActionID: "compare", Intent: "compare"},
			{ActionID: "ask", Intent: "ask"},
			{ActionID: "snooze", Intent: "snooze"},
		}
	case sovereignv1.LoopStage_LOOP_STAGE_DECIDE:
		seed = []opt{
			{ActionID: "open", Intent: "open"},
			{ActionID: "save", Intent: "save"},
			{ActionID: "ask", Intent: "ask"},
		}
	case sovereignv1.LoopStage_LOOP_STAGE_ACT:
		seed = []opt{
			{ActionID: "revisit", Intent: "revisit"},
			{ActionID: "ask", Intent: "ask"},
		}
	default:
		return nil
	}
	b, err := json.Marshal(seed)
	if err != nil {
		return nil
	}
	return b
}

func pickRenderDepth(bucket sovereignv1.SurfaceBucket) int32 {
	switch bucket {
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
		return 4
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
		return 2
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE:
		return 2
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW:
		return 1
	default:
		return 1
	}
}

func pickLoopPriority(bucket sovereignv1.SurfaceBucket) sovereignv1.LoopPriority {
	switch bucket {
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CRITICAL
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONTINUING
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_CONFIRM
	case sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE
	default:
		return sovereignv1.LoopPriority_LOOP_PRIORITY_REFERENCE
	}
}

// --- payload + key helpers --------------------------------------------------

type loopTransitionPayload struct {
	EntryKey   string
	LensModeID string
	FromStage  string
	ToStage    string
	Trigger    string
}

func parseLoopTransitionPayload(raw json.RawMessage) loopTransitionPayload {
	out := loopTransitionPayload{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	out.EntryKey = pickStringField(m, "entry_key")
	out.LensModeID = pickStringField(m, "lens_mode_id")
	out.FromStage = pickStringField(m, "from_stage")
	out.ToStage = pickStringField(m, "to_stage")
	out.Trigger = pickStringField(m, "trigger")
	return out
}

func mapStageNameOrFallback(name string, fallback sovereignv1.LoopStage) sovereignv1.LoopStage {
	switch name {
	case "LOOP_STAGE_OBSERVE":
		return sovereignv1.LoopStage_LOOP_STAGE_OBSERVE
	case "LOOP_STAGE_ORIENT":
		return sovereignv1.LoopStage_LOOP_STAGE_ORIENT
	case "LOOP_STAGE_DECIDE":
		return sovereignv1.LoopStage_LOOP_STAGE_DECIDE
	case "LOOP_STAGE_ACT":
		return sovereignv1.LoopStage_LOOP_STAGE_ACT
	}
	return fallback
}

func stageForLoopEvent(eventType string) sovereignv1.LoopStage {
	switch eventType {
	case EventKnowledgeLoopObserved,
		EventKnowledgeLoopReturned,
		EventKnowledgeLoopSessionReset:
		return sovereignv1.LoopStage_LOOP_STAGE_OBSERVE
	case EventKnowledgeLoopOriented,
		EventKnowledgeLoopDeferred:
		return sovereignv1.LoopStage_LOOP_STAGE_ORIENT
	case EventKnowledgeLoopDecisionPresented:
		return sovereignv1.LoopStage_LOOP_STAGE_DECIDE
	case EventKnowledgeLoopActed:
		return sovereignv1.LoopStage_LOOP_STAGE_ACT
	}
	return sovereignv1.LoopStage_LOOP_STAGE_OBSERVE
}

// keyFormat pins the canonical identifier format: alphanumeric plus _ : -, up to 128 chars.
var keyFormat = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

func deriveEntryKey(ev *sovereign_db.KnowledgeEvent) (string, error) {
	if key := extractStringField(ev.Payload, "entry_key", "item_key"); key != "" {
		if keyFormat.MatchString(key) {
			return key, nil
		}
	}
	if ev.AggregateType != "" && ev.AggregateID != "" {
		candidate := ev.AggregateType + ":" + sanitizeKeySegment(ev.AggregateID)
		if keyFormat.MatchString(candidate) {
			return candidate, nil
		}
	}
	return "event:" + ev.EventID.String(), nil
}

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

func extractArtifactVersionRef(payload json.RawMessage) *sovereignv1.KnowledgeLoopArtifactVersionRef {
	ref := &sovereignv1.KnowledgeLoopArtifactVersionRef{}
	if len(payload) == 0 {
		return ref
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ref
	}
	if v := pickStringField(m, "summary_version_id"); v != "" {
		ref.SummaryVersionId = &v
	}
	if v := pickStringField(m, "tag_set_version_id"); v != "" {
		ref.TagSetVersionId = &v
	}
	if v := pickStringField(m, "lens_version_id"); v != "" {
		ref.LensVersionId = &v
	}
	return ref
}

func pickStringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
