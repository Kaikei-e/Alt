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
	EventKnowledgeLoopSessionReset      = "knowledge_loop.session_reset.v1"
	EventKnowledgeLoopLensModeSwitched  = "knowledge_loop.lens_mode_switched.v1"
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
const WhyMappingVersion = 5
