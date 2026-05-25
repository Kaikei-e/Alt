package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

const (
	// projectorName is the row key used in projection_checkpoints to track how
	// far this projector has consumed the knowledge_events log.
	projectorName = "knowledge-loop-projector"
	// defaultBatchSize was 100 pre-ADR-000914. Reproject of a high-traffic
	// user produced O(N) macro_state recompute DB queries (one per Acted /
	// Reviewed / ActOutcome event), so the per-tick ceiling of
	// (batch_size / tick_interval) capped reproject throughput at ~20
	// events / sec. Bumped to 500 to amortise the per-tick checkpoint
	// round-trip; the per-event work is still bounded by
	// MaxBatchesPerTick × BatchSize before the goroutine yields.
	defaultBatchSize = 500
	// defaultMaxBatchesPerTick lets one wake-up drain multiple batches in
	// sequence when the log is backlogged. Mirrors the surface_planner_cron /
	// act_outcome_cron pattern which has env-tunable MaxBatchesPerTick.
	// Default is 4 so a quiet log still sleeps within tick_interval but a
	// reproject can chew through 2 000 events per 5 s tick before yielding.
	defaultMaxBatchesPerTick = 4
	defaultLensModeID        = "default"
)

// Repository is the subset of sovereign_db.Repository that the projector needs.
// Defining it here keeps the package unit-testable with an in-memory fake and
// documents the projector's only allowed coupling to durable state.
type Repository interface {
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	GetKnowledgeLoopEntries(ctx context.Context, filter sovereign_db.GetKnowledgeLoopEntriesFilter) ([]*sovereignv1.KnowledgeLoopEntry, error)
	UpsertKnowledgeLoopEntry(ctx context.Context, e *sovereignv1.KnowledgeLoopEntry) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopSessionState(ctx context.Context, s *sovereignv1.KnowledgeLoopSessionState) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopEntrySessionState(ctx context.Context, userID, tenantID, lensModeID, entryKey string, currentStage sovereignv1.LoopStage, currentStageEnteredAt time.Time, eventSeq int64) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	UpsertKnowledgeLoopSurface(ctx context.Context, s *sovereignv1.KnowledgeLoopSurface) (*sovereign_db.KnowledgeLoopUpsertResult, error)
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
	// PatchKnowledgeLoopEntryReviewLifecycle updates dismiss_state, visibility_state,
	// and completion_state in one statement, with all three states supplied
	// explicitly. Used by the KnowledgeLoopReviewed projector branch so the
	// trigger semantics (recheck / archive / mark_reviewed) are not collapsed
	// through a flat dismiss → visibility lookup. Other columns are preserved
	// and the same seq-hiwater guard applies as for PatchKnowledgeLoopEntryDismissState.
	PatchKnowledgeLoopEntryReviewLifecycle(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, dismissState sovereignv1.DismissState, visibilityState sovereignv1.LoopVisibilityState, completionState sovereignv1.LoopCompletionState) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// PatchKnowledgeLoopEntrySurfacePlan updates only planner-owned placement
	// columns on an existing entry. It is used by the system-only
	// KnowledgeLoopSurfacePlanRecomputed event and must preserve why_*, lifecycle,
	// freshness, artifacts, and action metadata.
	PatchKnowledgeLoopEntrySurfacePlan(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, surfaceBucket sovereignv1.SurfaceBucket, renderDepthHint int32, loopPriority sovereignv1.LoopPriority, plannerVersion sovereignv1.SurfacePlannerVersion, scoreInputs []byte) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// PatchKnowledgeLoopActTargetSourceURL fills act_targets[0].source_url for an
	// existing entry whose seed event predated the producer-side URL injection
	// added in ADR-000879. Driven by the corrective ArticleUrlBackfilled event:
	// touches only the JSONB key, preserves dismiss_state, why_*, freshness_at,
	// surface_bucket, and the rest of act_targets[0]. Idempotent at the SQL
	// boundary (a `NOT (act_targets->0 ? 'source_url')` predicate makes a
	// re-applied event a no-op once the URL is in place).
	PatchKnowledgeLoopActTargetSourceURL(ctx context.Context, userID, tenantID, lensModeID, entryKey, articleID, sourceURL string, eventSeq int64) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// PatchKnowledgeLoopEntryContinueContext patches only continue_context for an
	// existing entry, preserving every other column. Driven by Phase 2 semantic
	// knowledge_loop.acted.v1 events: the projector derives a bounded
	// `recent_action_labels` list from the event payload (no projection-row read)
	// and writes it back. Reproject-safe — the body is a pure function of
	// already-seen events ≤ eventSeq.
	PatchKnowledgeLoopEntryContinueContext(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, continueContext []byte) (*sovereign_db.KnowledgeLoopUpsertResult, error)
	// ListKnowledgeLoopActedEventsForEntry returns the most recent acted events
	// for (user, lens_mode, entry) at-or-before `untilSeq`, descending by seq,
	// limited to `limit` rows. Used by continue_context_builder to derive the
	// bounded recent_action_labels purely from the event log, never from the
	// projection row.
	ListKnowledgeLoopActedEventsForEntry(ctx context.Context, userID, tenantID uuid.UUID, lensModeID, entryKey string, untilSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)

	// ListKnowledgeEventsForUserInWindow returns all events of the given types
	// scoped to a user whose occurred_at falls in [since, until). Used by the
	// macro_state_builder branch to reduce the 7d window of acted / reviewed /
	// act_outcome events into the knowledge_loop_macro_state projection. The
	// window edges are caller-supplied (event-time bound) so reproject is
	// deterministic regardless of wall-clock.
	ListKnowledgeEventsForUserInWindow(ctx context.Context, userID uuid.UUID, eventTypes []string, since, until time.Time, limit int) ([]sovereign_db.KnowledgeEvent, error)

	// UpsertKnowledgeLoopMacroState writes the 7d macro projection row for
	// (user, tenant, lens). Merge-safe: the driver enforces a
	// `seq_hiwater <= EXCLUDED.seq_hiwater` guard so out-of-order replays
	// are no-ops. ADR-000909 §Δ2 supplement (2026-05-24).
	UpsertKnowledgeLoopMacroState(ctx context.Context, row sovereign_db.KnowledgeLoopMacroStateRow) (*sovereign_db.KnowledgeLoopUpsertResult, error)
}

// Config tunes the projector loop. Zero values fall back to defaults.
type Config struct {
	BatchSize int
	// MaxBatchesPerTick caps how many consecutive `BatchSize`-sized batches
	// one `RunBatch` invocation will drain before yielding back to the
	// scheduling goroutine. The default (4) keeps steady-state traffic
	// bounded; operators bump it via env during reproject to clear backlog
	// without changing the tick interval. A single-batch tick is restored
	// by setting it to 1. Zero falls back to the default.
	MaxBatchesPerTick int
}

// Projector runs the Knowledge Loop projection job over knowledge_events.
//
// Reproject-safety invariants (see docs/plan/knowledge-loop-canonical-contract.md):
//   - Entry/session state derives from event payloads; surface summaries are
//     deterministic compactions over the disposable Loop entry projection.
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
	if cfg.MaxBatchesPerTick <= 0 {
		cfg.MaxBatchesPerTick = defaultMaxBatchesPerTick
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

// RunBatch drains up to `cfg.MaxBatchesPerTick` consecutive batches from
// the projection checkpoint forward. The caller schedules invocations
// (typically every few seconds); each invocation walks one or more
// `cfg.BatchSize`-sized batches until either the log is drained or the
// per-tick cap is hit, then yields back to its goroutine. ADR-000914 §3
// — pre-bump the projector capped reproject throughput at 20 events / sec
// (BatchSize=100 × tick=5s) which was insufficient for the
// macro_state recompute work added by ADR-000911.
//
// Errors are logged and the checkpoint advances only across the events
// successfully read; bad individual events are skipped without stalling
// the whole projector. Context cancellation between batches is honoured
// so a shutdown signal is observed within one batch's worth of work.
func (p *Projector) RunBatch(ctx context.Context) error {
	batches := 0
	totalEvents := 0
	totalProjected := 0
	totalSkipped := 0

	for batches < p.cfg.MaxBatchesPerTick {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastSeq, err := p.repo.GetProjectionCheckpoint(ctx, projectorName)
		if err != nil {
			return fmt.Errorf("knowledge_loop_projector: get checkpoint: %w", err)
		}

		events, err := p.repo.ListKnowledgeEventsSince(ctx, lastSeq, p.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("knowledge_loop_projector: list events: %w", err)
		}
		if len(events) == 0 {
			// Log drained — yield back to the scheduling goroutine.
			break
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

		batches++
		totalEvents += len(events)
		totalProjected += projected
		totalSkipped += skipped

		// Short-batch — caller's log is drained or the page boundary is
		// reached. Either way, no point spinning another batch within the
		// same tick.
		if len(events) < p.cfg.BatchSize {
			break
		}
	}

	if totalEvents == 0 {
		return nil
	}

	p.logger.InfoContext(ctx, "knowledge_loop_projector: tick complete",
		slog.String("projector", projectorName),
		slog.Int("batches", batches),
		slog.Int("events", totalEvents),
		slog.Int("projected", totalProjected),
		slog.Int("skipped_by_guard", totalSkipped),
		slog.Int("max_batches_per_tick", p.cfg.MaxBatchesPerTick),
		slog.Int("batch_size", p.cfg.BatchSize),
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
		bucket, inputs, plannerVersion := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs, plannerVersion)
		if err != nil {
			return nil, err
		}
		res, err := p.repo.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return res, err
		}
		return res, p.recomputeSurfaces(ctx, ev, lensModeID)

	case EventSummaryNarrativeBackfilled:
		// ADR-000846: discovered event repairs historic entries' why_text via
		// the patch path. The full UPSERT is intentionally avoided — it would
		// overwrite dismiss_state and other entry-level fields that the user
		// (or earlier events) may have transitioned. The patch SQL touches
		// only the why_* columns; seq-hiwater guard ensures replay safety.
		return p.projectSummaryNarrativeBackfilled(ctx, ev, lensModeID)

	case EventArticleUrlBackfilled:
		// ADR-000879: corrective event repairs legacy Loop entries whose seed
		// event predated producer-side URL injection. The Loop projector
		// applies it as a JSONB patch on act_targets[0].source_url, preserving
		// every other field on the row. Reproject-safe: URL comes only from
		// payload; the driver's `NOT (act_targets->0 ? 'source_url')` predicate
		// keeps the patch idempotent across replays.
		return p.projectArticleUrlBackfilled(ctx, ev, lensModeID)

	case EventKnowledgeLoopSurfacePlanRecomputed:
		return p.projectSurfacePlanRecomputed(ctx, ev, lensModeID)

	case EventHomeItemOpened:
		bucket, inputs, plannerVersion := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_ACT,
			bucket, inputs, plannerVersion)
		if err != nil {
			return nil, err
		}
		entry.DismissState = sovereignv1.DismissState_DISMISS_STATE_COMPLETED
		entry.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE
		entry.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED
		entry.ContinueContext = buildContinueContextJSON(ev)
		res, err := p.repo.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return nil, err
		}
		if _, entryStateErr := p.repo.UpsertKnowledgeLoopEntrySessionState(
			ctx,
			ev.UserID.String(),
			ev.TenantID.String(),
			lensModeID,
			entry.EntryKey,
			sovereignv1.LoopStage_LOOP_STAGE_ACT,
			ev.OccurredAt,
			ev.EventSeq,
		); entryStateErr != nil {
			return res, fmt.Errorf("entry session upsert after opened: %w", entryStateErr)
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
		return res, p.recomputeSurfaces(ctx, ev, lensModeID)

	case EventHomeItemDismissed:
		bucket, inputs, plannerVersion := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs, plannerVersion)
		if err != nil {
			return nil, err
		}
		entry.DismissState = sovereignv1.DismissState_DISMISS_STATE_DISMISSED
		entry.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE
		entry.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_DISMISSED
		entry.DecisionOptions = nil
		res, err := p.repo.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return res, err
		}
		return res, p.recomputeSurfaces(ctx, ev, lensModeID)

	case EventHomeItemSuperseded, EventSummarySuperseded:
		bucket, inputs, plannerVersion := p.resolveBucketAndInputs(ctx, ev)
		entry, err := p.buildEntryFromEvent(ev, lensModeID,
			sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			bucket, inputs, plannerVersion)
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
		res, err := p.repo.UpsertKnowledgeLoopEntry(ctx, entry)
		if err != nil {
			return res, err
		}
		return res, p.recomputeSurfaces(ctx, ev, lensModeID)

	case EventKnowledgeLoopObserved,
		EventKnowledgeLoopOriented,
		EventKnowledgeLoopDecisionPresented,
		EventKnowledgeLoopActed,
		EventKnowledgeLoopReturned,
		EventKnowledgeLoopDeferred,
		EventKnowledgeLoopReviewed,
		EventKnowledgeLoopSessionReset,
		EventKnowledgeLoopLensModeSwitched:
		return p.projectTransition(ctx, ev)

	case EventKnowledgeLoopInternalized:
		// ADR-000914: "I got this" graduation. Same-stage transition that
		// only flips dismiss_state to INTERNALIZED; the OODA stage and
		// session_state cursor are untouched. Reuses the patch-only
		// driver path so freshness_at / why_text / surface_bucket are
		// preserved (same discipline as the Deferred branch above).
		return p.projectInternalized(ctx, ev)

	case EventKnowledgeLoopActOutcome:
		// ADR-000908 §Δ1: outcome events are not entry-producing — the
		// signal they carry is aggregated by EventLogSurfaceScoreResolver
		// on the next entry projection (ActOutcomeSignal). The projector
		// only emits the counter so dashboards / alert rules can track
		// coverage. The outcome label is normalised to the bounded enum
		// vocabulary so cardinality stays tractable.
		observeActOutcomeEmitted(normaliseActOutcomeLabel(extractStringField(ev.Payload, "outcome")))

		// ADR-000909 §Δ2 supplement: act_outcome=internalized changes the
		// graduation count and removes the entry from active_continue. Other
		// outcomes do not move the macro counts, but we recompute on every
		// outcome event so the byline reflects the latest snapshot — the
		// builder is pure and the upsert is merge-safe, so the additional
		// write amplification is bounded by the seq-hiwater guard.
		if err := p.recomputeMacroState(ctx, ev, lensModeID); err != nil {
			p.logger.WarnContext(ctx, "knowledge_loop_projector: recompute macro state after act_outcome failed",
				slog.Int64("event_seq", ev.EventSeq),
				slog.String("err", err.Error()),
			)
		}
		return nil, nil

	default:
		return nil, nil
	}
}

// normaliseActOutcomeLabel maps an outcome payload field onto a bounded
// metric label. Unknown values become "unspecified" so a typo or
// schema-drifted outcome cannot blow up Prometheus cardinality.
func normaliseActOutcomeLabel(raw string) string {
	switch raw {
	case "engaged", "ACT_OUTCOME_KIND_ENGAGED":
		return "engaged"
	case "deep_engagement", "ACT_OUTCOME_KIND_DEEP_ENGAGEMENT":
		return "deep_engagement"
	case "accepted_change", "ACT_OUTCOME_KIND_ACCEPTED_CHANGE":
		return "accepted_change"
	case "stale_save", "ACT_OUTCOME_KIND_STALE_SAVE":
		return "stale_save"
	case "no_engagement", "ACT_OUTCOME_KIND_NO_ENGAGEMENT":
		return "no_engagement"
	default:
		return "unspecified"
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

// projectArticleUrlBackfilled handles the corrective ArticleUrlBackfilled
// event emitted by alt-backend's knowledge-url-backfill job (ADR-000867).
// The Loop projector consumes the same event the home projector uses (see
// alt-backend job/knowledge_projector.go projectArticleUrlBackfilled) and
// applies it as a JSONB patch on knowledge_loop_entries.act_targets[0]
// .source_url so legacy entries — created before producer-side URL injection
// shipped (ADR-000879) — recover their external HTTPS URL without a full
// reproject sweep.
//
// Reproject-safe / merge-safe:
//   - URL comes strictly from event payload; never reads latest article state.
//   - http/https allowlist applied here (defense-in-depth — the SQL boundary
//     also rejects empty URL).
//   - Patch is a JSONB-key write; dismiss_state, why_*, freshness_at,
//     surface_bucket, and the rest of act_targets[0] stay untouched.
//   - Idempotent at the SQL boundary via `NOT (act_targets->0 ? 'source_url')`
//     so a replayed event becomes a no-op once the URL is populated.
func (p *Projector) projectArticleUrlBackfilled(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
	lensModeID string,
) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	if ev.UserID == nil {
		return nil, nil
	}
	articleID := extractStringField(ev.Payload, "article_id")
	if articleID == "" {
		return nil, nil
	}
	rawURL := extractStringField(ev.Payload, "url", "link")
	if rawURL == "" {
		return nil, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return nil, nil
	}
	entryKey := "article:" + articleID
	return p.repo.PatchKnowledgeLoopActTargetSourceURL(
		ctx,
		ev.UserID.String(),
		ev.TenantID.String(),
		lensModeID,
		entryKey,
		articleID,
		rawURL,
		ev.EventSeq,
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
	if entryKey != "" {
		if _, entrySessionErr := p.repo.UpsertKnowledgeLoopEntrySessionState(
			ctx,
			ev.UserID.String(),
			ev.TenantID.String(),
			lensModeID,
			entryKey,
			state.CurrentStage,
			ev.OccurredAt,
			ev.EventSeq,
		); entrySessionErr != nil {
			return sessionRes, fmt.Errorf("entry session upsert on transition: %w", entrySessionErr)
		}
	}

	// Deferred uniquely flips the entry's dismiss_state to DEFERRED. The
	// Review-lane KnowledgeLoopReviewed event also flips dismiss_state, but
	// to a value chosen by the trigger sub-field on the event payload:
	//   recheck       → ACTIVE   (re-arms the entry; freshness is already
	//                              the event.OccurredAt via the projection
	//                              pipeline, so the next snapshot promotes
	//                              it back into the foreground)
	//   archive       → COMPLETED
	//   mark_reviewed → COMPLETED
	// All other transitions leave the entry row untouched and only update
	// session state above.
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
	if ev.EventType == EventKnowledgeLoopReviewed && entryKey != "" {
		lc := reviewLifecycleForReviewedEvent(ev.Payload)
		if _, patchErr := p.repo.PatchKnowledgeLoopEntryReviewLifecycle(
			ctx,
			ev.UserID.String(),
			ev.TenantID.String(),
			lensModeID,
			entryKey,
			ev.EventSeq,
			lc.DismissState,
			lc.VisibilityState,
			lc.CompletionState,
		); patchErr != nil {
			return sessionRes, fmt.Errorf("patch review lifecycle on Reviewed: %w", patchErr)
		}
	}

	// Phase 2 semantic feedback loop: when the Acted event carries an
	// acted_intent, derive a bounded recent_action_labels list purely from
	// the event log (≤ this event's seq) and patch continue_context. The
	// builder is pure; reproject reproduces the same JSONB from the same
	// log slice.
	if ev.EventType == EventKnowledgeLoopActed && entryKey != "" && payload.ActedIntent != "" && ev.UserID != nil {
		recent, listErr := p.repo.ListKnowledgeLoopActedEventsForEntry(
			ctx,
			*ev.UserID,
			ev.TenantID,
			lensModeID,
			entryKey,
			ev.EventSeq,
			recentActionLabelsBound,
		)
		if listErr != nil {
			p.logger.WarnContext(ctx, "knowledge_loop_projector: list acted events for continue_context failed",
				"event_seq", ev.EventSeq,
				"entry_key", entryKey,
				"err", listErr,
			)
		} else if body := buildContinueContextFromActed(recent); body != nil {
			if _, patchErr := p.repo.PatchKnowledgeLoopEntryContinueContext(
				ctx,
				ev.UserID.String(),
				ev.TenantID.String(),
				lensModeID,
				entryKey,
				ev.EventSeq,
				body,
			); patchErr != nil {
				return sessionRes, fmt.Errorf("patch continue_context on Acted: %w", patchErr)
			}
		}
	}

	// ADR-000909 §Δ2 supplement: recompute the macro projection when the
	// transition is one that changes the user's 7d footprint. Acted/Reviewed
	// land here directly; Deferred + Returned do not affect the macro counts
	// in Phase A, so we skip them to keep the per-event write amplification
	// bounded.
	if ev.EventType == EventKnowledgeLoopActed || ev.EventType == EventKnowledgeLoopReviewed {
		if err := p.recomputeMacroState(ctx, ev, lensModeID); err != nil {
			p.logger.WarnContext(ctx, "knowledge_loop_projector: recompute macro state failed",
				slog.Int64("event_seq", ev.EventSeq),
				slog.String("event_type", ev.EventType),
				slog.String("err", err.Error()),
			)
		}
	}

	return sessionRes, p.recomputeSurfaces(ctx, ev, lensModeID)
}

// dismissStateForReviewedEvent has moved to review_lifecycle.go alongside
// reviewLifecycleForReviewedEvent so the trigger semantics for recheck /
// archive / mark_reviewed live in one place. New code should call
// reviewLifecycleForReviewedEvent directly to avoid the dismiss-state /
// visibility-state collapse that conflated archive (hide) with mark_reviewed
// (keep visible in Review).

func (p *Projector) recomputeSurfaces(ctx context.Context, ev *sovereign_db.KnowledgeEvent, lensModeID string) error {
	if ev.UserID == nil {
		return nil
	}
	userID := ev.UserID.String()
	tenantID := ev.TenantID.String()
	for _, bucket := range []sovereignv1.SurfaceBucket{
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW,
	} {
		b := bucket
		entries, err := p.repo.GetKnowledgeLoopEntries(ctx, sovereign_db.GetKnowledgeLoopEntriesFilter{
			TenantID:      ev.TenantID,
			UserID:        *ev.UserID,
			LensModeID:    lensModeID,
			SurfaceBucket: &b,
			Limit:         3,
		})
		if err != nil {
			return fmt.Errorf("recompute surface %s: %w", bucket.String(), err)
		}

		var primary *string
		secondary := make([]string, 0, 2)
		if len(entries) > 0 {
			key := entries[0].EntryKey
			primary = &key
			for _, e := range entries[1:] {
				if len(secondary) >= 2 {
					break
				}
				secondary = append(secondary, e.EntryKey)
			}
		}

		health, _ := json.Marshal(map[string]int{"active_count": len(entries)})
		surface := &sovereignv1.KnowledgeLoopSurface{
			UserId:               userID,
			TenantId:             tenantID,
			LensModeId:           lensModeID,
			SurfaceBucket:        bucket,
			PrimaryEntryKey:      primary,
			SecondaryEntryKeys:   secondary,
			ProjectionSeqHiwater: ev.EventSeq,
			FreshnessAt:          timestamppb.New(ev.OccurredAt),
			ServiceQuality:       sovereignv1.LoopServiceQuality_LOOP_SERVICE_QUALITY_FULL,
			LoopHealth:           health,
		}
		if _, err := p.repo.UpsertKnowledgeLoopSurface(ctx, surface); err != nil {
			return fmt.Errorf("upsert surface %s: %w", bucket.String(), err)
		}
	}
	return nil
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
	plannerVersion sovereignv1.SurfacePlannerVersion,
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

	entry := &sovereignv1.KnowledgeLoopEntry{
		UserId:                ev.UserID.String(),
		TenantId:              ev.TenantID.String(),
		LensModeId:            lensModeID,
		EntryKey:              entryKey,
		SourceItemKey:         sourceItemKey,
		ProposedStage:         proposedStage,
		SurfaceBucket:         surfaceBucket,
		ProjectionSeqHiwater:  ev.EventSeq,
		SourceEventSeq:        ev.EventSeq,
		FreshnessAt:           timestamppb.New(ev.OccurredAt),
		SourceObservedAt:      timestamppb.New(eventObservedAt(ev)),
		ArtifactVersionRef:    art,
		WhyPrimary:            why,
		DecisionOptions:       seedDecisionOptions(proposedStage),
		ActTargets:            seedActTargets(ev, inputs),
		DismissState:          sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
		VisibilityState:       sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
		CompletionState:       sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN,
		RenderDepthHint:       pickRenderDepth(surfaceBucket),
		LoopPriority:          pickLoopPriority(surfaceBucket),
		SurfacePlannerVersion: plannerVersion.Enum(),
	}
	if plannerVersion == sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2 {
		entry.SurfaceScoreInputs = marshalSurfaceScoreInputs(inputs)
	}
	// ADR-000907: review_reason is derived from the same SurfaceScoreInputs
	// the bucket decision used, so reproject converges to the same value
	// without consulting latest projection state.
	entry.ReviewReason = decideReviewReason(inputs)
	return entry, nil
}

// seedActTargets materializes the act_targets[] JSON the projector writes onto
// the entry. The downstream chain — knowledge_loop_entries.act_targets (JSONB)
// → alt-backend's decodeActTargets → loopv1.ActTarget — uses the same JSON
// shape decision_options uses, so we marshal `[{target_type, target_ref,
// route}]` as plain JSON bytes here.
//
// For Recap: the route template is constant (`/recap/topic/<id>`); the
// snapshot id is supplied by the resolver, which has already validated it as
// a UUID. The frontend additionally guards against non-relative routes when
// rendering the CTA, so even a future resolver bug cannot smuggle a
// `javascript:` scheme through this seam.
//
// Reproject-safe: pure function of `inputs`, which is itself derived from
// event payload + versioned artifacts only.
func seedActTargets(ev *sovereign_db.KnowledgeEvent, inputs SurfaceScoreInputs) []byte {
	type actTarget struct {
		TargetType string `json:"target_type"`
		TargetRef  string `json:"target_ref"`
		Route      string `json:"route,omitempty"`
		SourceURL  string `json:"source_url,omitempty"`
	}
	out := []actTarget{}
	if articleID := articleActTargetID(ev); articleID != "" {
		out = append(out, actTarget{
			TargetType: "article",
			TargetRef:  articleID,
			Route:      "/articles/" + url.PathEscape(articleID),
			SourceURL:  articleActSourceURL(ev),
		})
	}
	if inputs.RecapTopicSnapshotID != "" {
		out = append(out, actTarget{
			TargetType: "recap",
			TargetRef:  inputs.RecapTopicSnapshotID,
			Route:      "/recap/topic/" + inputs.RecapTopicSnapshotID,
		})
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// articleActSourceURL extracts the article's external HTTPS source URL from
// the event payload. Reads the canonical "url" key first; falls back to the
// legacy "link" key (PM-2026-041 historic events). Only http(s) schemes pass;
// javascript:/data:/file:/relative/etc. are rejected to defense-in-depth the
// FE's safeArticleHref guard.
//
// Reproject-safe: pure function of `ev.Payload`. Never reads latest article
// state; the article URL is treated as immutable per article_id, and producers
// must include it in the event payload at append time.
func articleActSourceURL(ev *sovereign_db.KnowledgeEvent) string {
	if ev == nil {
		return ""
	}
	raw := extractStringField(ev.Payload, "url", "link")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if parsed.Host == "" {
		return ""
	}
	return raw
}

func articleActTargetID(ev *sovereign_db.KnowledgeEvent) string {
	if ev == nil {
		return ""
	}
	if id := extractStringField(ev.Payload, "article_id"); id != "" {
		return id
	}
	if ev.AggregateType == "article" && ev.AggregateID != "" {
		return ev.AggregateID
	}
	if key := extractStringField(ev.Payload, "entry_key", "item_key"); strings.HasPrefix(key, "article:") {
		return strings.TrimPrefix(key, "article:")
	}
	return ""
}

func marshalSurfaceScoreInputs(in SurfaceScoreInputs) []byte {
	type surfaceScoreInputsJSON struct {
		TopicOverlapCount         uint32 `json:"topic_overlap_count"`
		TagOverlapCount           uint32 `json:"tag_overlap_count"`
		HasAugurLink              bool   `json:"has_augur_link"`
		VersionDriftCount         uint32 `json:"version_drift_count"`
		HasOpenInteraction        bool   `json:"has_open_interaction"`
		FreshnessAt               string `json:"freshness_at,omitempty"`
		EventType                 string `json:"event_type"`
		RecapTopicSnapshotID      string `json:"recap_topic_snapshot_id,omitempty"`
		EvidenceDensity           uint32 `json:"evidence_density,omitempty"`
		RecapClusterMomentum      uint32 `json:"recap_cluster_momentum,omitempty"`
		QuestionContinuationScore uint32 `json:"question_continuation_score,omitempty"`
		ReportWorthinessScore     uint32 `json:"report_worthiness_score,omitempty"`
		StalenessScore            uint32 `json:"staleness_score,omitempty"`
		ContradictionCount        uint32 `json:"contradiction_count,omitempty"`
	}
	out := surfaceScoreInputsJSON{
		TopicOverlapCount:         in.TopicOverlapCount,
		TagOverlapCount:           in.TagOverlapCount,
		HasAugurLink:              in.HasAugurLink,
		VersionDriftCount:         in.VersionDriftCount,
		HasOpenInteraction:        in.HasOpenInteraction,
		EventType:                 in.EventType,
		RecapTopicSnapshotID:      in.RecapTopicSnapshotID,
		EvidenceDensity:           in.EvidenceDensity,
		RecapClusterMomentum:      in.RecapClusterMomentum,
		QuestionContinuationScore: in.QuestionContinuationScore,
		ReportWorthinessScore:     in.ReportWorthinessScore,
		StalenessScore:            in.StalenessScore,
		ContradictionCount:        in.ContradictionCount,
	}
	if !in.FreshnessAt.IsZero() {
		out.FreshnessAt = in.FreshnessAt.UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
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
	// Phase 2 semantic Decide / Act feedback loop. The projector reads these
	// to update continue_context.recent_action_labels, act_targets, and the
	// Surface Planner v2 Continue signal — all from event payload only, so
	// reproject reproduces the same projection from the same event log.
	ActedIntent      string
	ActionID         string
	TargetType       string
	TargetRef        string
	ContinueFlag     bool
	HasContinueFlag  bool
	PresentedIntents []string
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
	out.ActedIntent = pickStringField(m, "acted_intent")
	out.ActionID = pickStringField(m, "action_id")
	out.TargetType = pickStringField(m, "target_type")
	out.TargetRef = pickStringField(m, "target_ref")
	if v, ok := m["continue_flag"].(bool); ok {
		out.ContinueFlag = v
		out.HasContinueFlag = true
	}
	if raw, ok := m["presented_intents"].([]any); ok {
		out.PresentedIntents = make([]string, 0, len(raw))
		for _, x := range raw {
			if s, ok := x.(string); ok && s != "" {
				out.PresentedIntents = append(out.PresentedIntents, s)
			}
		}
	}
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

func eventObservedAt(ev *sovereign_db.KnowledgeEvent) time.Time {
	if ev == nil {
		return time.Time{}
	}
	if t := readPayloadTimestamp(ev.Payload,
		"source_observed_at",
		"published_at",
		"observed_at",
		"opened_at",
		"dismissed_at",
		"linked_at",
	); !t.IsZero() {
		return t
	}
	return ev.OccurredAt
}

func readPayloadTimestamp(payload json.RawMessage, keys ...string) time.Time {
	s := extractStringField(payload, keys...)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
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
