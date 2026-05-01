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

	EventKnowledgeLoopObserved          = "knowledge_loop.observed.v1"
	EventKnowledgeLoopOriented          = "knowledge_loop.oriented.v1"
	EventKnowledgeLoopDecisionPresented = "knowledge_loop.decision_presented.v1"
	EventKnowledgeLoopActed             = "knowledge_loop.acted.v1"
	EventKnowledgeLoopReturned          = "knowledge_loop.returned.v1"
	EventKnowledgeLoopDeferred          = "knowledge_loop.deferred.v1"
	EventKnowledgeLoopReviewed          = "knowledge_loop.reviewed.v1"
	EventKnowledgeLoopSessionReset      = "knowledge_loop.session_reset.v1"
	EventKnowledgeLoopLensModeSwitched  = "knowledge_loop.lens_mode_switched.v1"

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
const WhyMappingVersion = 8
