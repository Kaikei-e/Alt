package knowledge_loop_projector

// Event type names mirror the canonical knowledge_event.event_type vocabulary
// (originally defined in alt-backend/app/domain/knowledge_event.go). The
// projector only needs the string literals — the source of truth for emission
// remains the producers (alt-backend, pre-processor) until/unless that ownership
// also moves into knowledge-sovereign.
const (
	EventArticleCreated        = "ArticleCreated"
	EventSummaryVersionCreated = "SummaryVersionCreated"
	EventHomeItemsSeen         = "HomeItemsSeen"
	EventHomeItemOpened        = "HomeItemOpened"
	EventHomeItemDismissed     = "HomeItemDismissed"
	EventHomeItemAsked         = "HomeItemAsked"
	EventSummarySuperseded     = "SummarySuperseded"
	EventHomeItemSuperseded    = "HomeItemSuperseded"

	// EventSummaryNarrativeBackfilled is the discovered event emitted by
	// alt-backend's summary-narrative-backfill job to repair Knowledge Loop
	// entries whose original SummaryVersionCreated event lacks article_title
	// in payload. The projector handles this event with a patch-only-why path
	// that preserves dismiss_state and other entry fields. See ADR-000846.
	EventSummaryNarrativeBackfilled = "SummaryNarrativeBackfilled"

	// EventArticleUrlBackfilled is the corrective event emitted by
	// alt-backend's knowledge-url-backfill job (ADR-000867 / ADR-000879).
	// Its payload carries `article_id` + `url` and the Loop projector applies
	// it as a patch-only path to fill act_targets[].source_url on legacy
	// projection rows whose seed event predated producer-side URL injection.
	// dismiss_state, why_*, freshness_at, surface_bucket are preserved by
	// the dedicated patch SQL.
	EventArticleUrlBackfilled = "ArticleUrlBackfilled"

	EventKnowledgeLoopObserved          = "knowledge_loop.observed.v1"
	EventKnowledgeLoopOriented          = "knowledge_loop.oriented.v1"
	EventKnowledgeLoopDecisionPresented = "knowledge_loop.decision_presented.v1"
	EventKnowledgeLoopActed             = "knowledge_loop.acted.v1"
	EventKnowledgeLoopReturned          = "knowledge_loop.returned.v1"
	EventKnowledgeLoopDeferred          = "knowledge_loop.deferred.v1"
	EventKnowledgeLoopReviewed          = "knowledge_loop.reviewed.v1"
	EventKnowledgeLoopSessionReset      = "knowledge_loop.session_reset.v1"
	EventKnowledgeLoopLensModeSwitched  = "knowledge_loop.lens_mode_switched.v1"

	// ADR-000914: "I got this" graduation producer. The projector flips
	// dismiss_state to DISMISS_STATE_INTERNALIZED via
	// PatchKnowledgeLoopEntryDismissState; downstream read paths already
	// filter internalized rows so the entry disappears from foreground /
	// Continue / Now without touching the event log.
	EventKnowledgeLoopInternalized = "knowledge_loop.internalized.v1"

	// ADR-000908 §Δ1 — system-emitted closure signal for a prior Acted
	// event. Two producers exist: alt-backend view trackers emit
	// engaged / deep_engagement immediately when dwell or conversation-turn
	// thresholds clear, and knowledge-sovereign's act_outcome_cron emits
	// no_engagement after a 7-day event-time window expires without an
	// explicit outcome. Consumed by the projector (metrics-only branch)
	// and the EventLogSurfaceScoreResolver (ActOutcomeSignal aggregation).
	EventKnowledgeLoopActOutcome = "knowledge_loop.act_outcome.v1"

	// Upstream snapshot events feeding Surface Planner v2. Emitted by
	// recap-worker, augur, and knowledge-sovereign-internal respectively.
	// The projector recognises them so a real SurfaceScoreResolver (Wave 4)
	// can subscribe to them via the same event log; until then the projector
	// silently no-ops on them so an early emitter doesn't break the batch.
	// See canonical contract §6.4.1.
	EventRecapTopicSnapshotted              = "recap.topic_snapshotted.v1"
	EventAugurConversationLinked            = "augur.conversation_linked.v1"
	EventKnowledgeLoopSurfacePlanRecomputed = "knowledge_loop.surface_plan_recomputed.v1"
)

// Aggregate type for Knowledge Loop session-state aggregates.
const AggregateLoopSession = "knowledge_loop_session"

// WhyMappingVersion is the exhaustive-mapping-table version for Phase-0 why
// codes → WhyKind. Bump this constant when the mapping changes; a bump
// triggers a full reproject via runbook.
//
// v3 (2026-04-26): why_text rewritten from placeholder strings to substantive
// narratives that explain why the entry is on the user's loop. Stage-appropriate
// seedDecisionOptions replaces the previous Source/Observe-only block.
//
// v4 (2026-04-26): projector ownership moved to knowledge-sovereign; runtime
// behavior unchanged from v3 but the bump signals operators that the projection
// is now driven from this service rather than the alt-backend job runner.
//
// v5 (2026-04-26): SummaryNarrativeBackfilled event type added so historic
// entries whose original SummaryVersionCreated event lacked article_title
// can be patched with a real narrative. Bump is a runbook signal that
// operators may optionally trigger a full reproject after backfill completes
// to verify replay convergence. ADR-000846.
//
// v6 (2026-04-26): EventLogSurfaceScoreResolver wired via WithScoreResolver.
// Adds three new WhyKinds — `topic_affinity_why`, `tag_trending_why`,
// `unfinished_continue_why` — emitted by enricher when the resolver returns
// non-zero v2 evidence (recap topic overlap, tag set overlap, or augur
// link / open interaction). Bump triggers a full reproject so historic
// entries pick up the v2 placement and Why narrative deterministically.
// Wave 4-C wiring (ADR-000853).
//
// v7 (2026-04-27): Surface Planner v2 signal expansion (fb.md §B-2).
// Adds StalenessScore (pure function of event.OccurredAt - source_observed_at),
// ContradictionCount (= count of SummarySuperseded targeting the article in
// the score window), QuestionContinuationScore (count of
// AugurConversationLinked events for this entry), RecapClusterMomentum
// (count of overlapping RecapTopicSnapshotted events), and EvidenceDensity /
// ReportWorthinessScore (wire-ready, behavior-gated until Acolyte ships).
// decideBucketV2 priority order tightened so Review becomes a deliberate
// re-evaluation queue rather than a leftover bucket. Projector also seeds
// `act_targets[]` with a Recap entry when the resolver pinned a matching
// snapshot id. Bump triggers a full reproject (knowledge-loop-reproject
// runbook v7 row).
//
// v8 (2026-05-01): SurfacePlanRecomputed projector branch added. The
// system-only replan event now patches planner-owned entry placement columns
// (surface_bucket, render_depth_hint, loop_priority, planner version, score
// inputs) without touching why/lifecycle/freshness fields, then recomputes the
// four surfaces. Bump signals operators to include the new branch in replay
// validation.
//
// v9 (2026-05-09): why_override priority reordered to the canonical contract
// §11 ladder (change > unfinished_continue > topic_affinity > tag_trending >
// recall > source). RECALL was previously checked before topic / tag overlap;
// now it is the residual kind so a single prior open does not crowd out an
// active recap-cluster or tag-stream connection. Also: KnowledgeLoopReviewed
// projection split into recheck / archive / mark_reviewed lifecycle outcomes
// — mark_reviewed keeps the entry visible in Review (was hidden under v8).
// Bump triggers a full reproject; runbook history table updated. Phase 3 of
// docs/plan/knowledge-loop-completion-03-review-why-quality.md.
//
// v10 (2026-05-23): ADR-000908 §Δ1 ActOutcomeSignal lands as a bucket
// driver. EventLogSurfaceScoreResolver now aggregates
// knowledge_loop.act_outcome.v1 events on the entry inside the 7d window
// (engaged=+1, deep_engagement=+2, accepted_change=+1, stale_save=-1,
// no_engagement=-2) and decideBucketV2 demotes Now/Continue placements to
// Review when the cumulative signal is ≤ -2. CHANGED still outranks the
// demotion so version drift is never silently hidden. Bump triggers a full
// reproject; runbook history table will get the v9 → v10 row when the
// reproject cutover is scheduled.
//
// v11 (2026-05-25): ADR-000908 §Δ4 WhyPayload v2 producer wiring lands.
// EnrichWhyFromEvent + OverrideWhyFromSurfaceInputs now populate the
// counter_evidence_refs, confidence_ladder, and what_would_change_my_mind
// fields on every emitted WhyPayload via the pure helpers
// (boundCounterEvidence, confidenceLadderFromKind, whatWouldChangeFromKind).
// Sovereign proto + DB schema gain three additive columns; alt-backend BFF
// passes the new fields through to the alt.knowledge.loop.v1 wire types.
// Bump triggers a full reproject so all historical entries gain the v2
// fields deterministically.
const WhyMappingVersion = 11
