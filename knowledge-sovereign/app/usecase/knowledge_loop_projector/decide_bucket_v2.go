package knowledge_loop_projector

import (
	"time"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// SurfaceScoreInputs holds the reproject-safe evidence used by Surface
// Planner v2 to decide which bucket an entry lands in. Every field is either
// a count derived from versioned tables (immutable) or an event-time bound
// timestamp (no wall-clock). decideBucketV2 is therefore pure: same inputs
// always yield the same bucket regardless of when it runs.
//
// Fields:
//   - TopicOverlapCount: number of recap_topic_snapshot terms that overlap
//     with the entry's article tags / summary keywords inside
//     [event.occurred_at - 7d, event.occurred_at).
//   - TagOverlapCount: number of tag_set_versions emissions for this user
//     in the same window where one of the article's tags appears.
//   - HasAugurLink: true when an AugurConversationLinked event resolved this
//     entry to an open Augur thread.
//   - VersionDriftCount: number of summary_version supersedes affecting the
//     article since the user last opened it (or since article creation if
//     never opened). Counts versioned facts, never latest state.
//   - HasOpenInteraction: true when an EventHomeItemOpened anchors the entry
//     to a continuing thread.
//   - FreshnessAt: MAX(event.occurred_at) on the chain of events feeding the
//     entry. Used here as a tiebreak signal, not as a decay value — decay is
//     computed at render time, never stored.
//   - EventType: the canonical event type that produced the entry. Used as a
//     v1 fallback when no v2 evidence is present.
type SurfaceScoreInputs struct {
	TopicOverlapCount  uint32
	TagOverlapCount    uint32
	HasAugurLink       bool
	VersionDriftCount  uint32
	HasOpenInteraction bool
	FreshnessAt        time.Time
	EventType          string

	// RecapTopicSnapshotID is the canonical id of the most-recent
	// RecapTopicSnapshotted event whose top_terms overlap with this entry's
	// tags inside the score window. The resolver validates the id is a UUID
	// before exposing it; the projector formats `/recap/topic/<id>` from it
	// and seeds an act_target with target_type=recap. Empty when no
	// matching snapshot exists. Reproject-safe: derived from event payload
	// only, never from latest cross-table state.
	RecapTopicSnapshotID string

	// EvidenceDensity is the count of distinct evidence_ref_id strings the
	// enricher attached to the entry's why_primary. Zero is a strong "we
	// have no anchors" signal and tips the entry into Review. Reproject-safe
	// because evidence refs are derived from event payload + versioned ids.
	EvidenceDensity uint32

	// RecapClusterMomentum counts RecapTopicSnapshotted events in the score
	// window whose top_terms overlap with the entry. Differs from
	// TopicOverlapCount in semantic intent: TopicOverlapCount measures
	// per-event overlap (how often a recap touched this article's topics);
	// RecapClusterMomentum is the same number with a cleaner name fb.md
	// Phase B-2 maps to a Now-promotion priority. Kept separate so a future
	// resolver can distinguish "hot cluster" from "any overlap".
	RecapClusterMomentum uint32

	// QuestionContinuationScore counts AugurConversationLinked events in
	// the score window for this entry. Distinct from HasAugurLink (which is
	// a bool) because Phase B-2 wants count semantics so multiple parallel
	// threads can promote Continue. v1 definition: count of links — does
	// not infer "open vs closed" because no Resolved/Cancelled event exists
	// yet (immutable-design-guard finding 5).
	QuestionContinuationScore uint32

	// ReportWorthinessScore is reserved for the Acolyte integration. Until
	// AcolyteReportRequested / *Generated / *Reviewed events land we always
	// populate 0; decideBucketV2 ignores it. Wire-ready, behaviour-gated.
	ReportWorthinessScore uint32

	// StalenessScore is the pureStalenessBucket of (event.OccurredAt,
	// source_observed_at). 0 = fresh, 4 = ≥30 days. Bumps the Review
	// fallback when ≥ 2 (older than 7 days). Reproject-safe — derived from
	// event payload only.
	StalenessScore uint32

	// ContradictionCount is v1-defined as count(SummarySuperseded events
	// targeting this article in 7d). Strong "your understanding may be
	// wrong" signal, promotes Changed alongside VersionDriftCount.
	// immutable-design-guard finding 5: an LLM-based contradiction judge
	// is non-deterministic and would need its own SummaryContradicted
	// event before we can use it.
	ContradictionCount uint32

	// RecentContinueActionCount counts knowledge_loop.acted.v1 events with
	// continue_flag=true scoped to this entry inside the v2 score window
	// (7 days, event-time bound). Phase 2 semantic feedback signal: a user
	// who Open-ed / Ask-ed / Revisit-ed an entry within the last week is
	// continuing a thought, so the entry promotes to Continue regardless of
	// v1 mapping. Reproject-safe — derived from event payload only.
	RecentContinueActionCount uint32

	// ActOutcomeSignal is the cumulative downstream outcome score derived
	// from knowledge_loop.act_outcome.v1 events on the entry inside the v2
	// score window (ADR-000908 §Δ1). The aggregation table is:
	//
	//	engaged          = +1
	//	deep_engagement  = +2
	//	accepted_change  = +1
	//	stale_save       = -1
	//	no_engagement    = -2
	//
	// A strong negative cumulative (≤ -2) demotes Now/Continue placements to
	// Review so the Loop stops re-promoting content the user has actively
	// skipped. CHANGED still outranks the demotion because version drift
	// must always surface. Positive values are used as within-bucket ranking
	// hints; they do not change bucket selection. Reproject-safe — derived
	// from event payload only.
	ActOutcomeSignal int32

	// ConfidenceLadder is the persist-stage confidence the recap-worker
	// computed for the originating topic cluster (ADR-000913 §D-10,
	// Bayesian RAG grounding). The recap pipeline emits a per-cluster ladder
	// (speculation / pattern / evidence / verified) based on evidence
	// density and soft-failure ratios; the projector reads it from the
	// surface_plan_recomputed event payload and uses SPECULATION to demote
	// Now/Continue placements to Review (same priority as ActOutcomeSignal
	// ≤ -2). Reproject-safe — comes from event payload, not latest state.
	ConfidenceLadder int32
}

// decideBucketV2 picks a SurfaceBucket from the score inputs. The order
// below encodes the canonical contract §6 priority: Changed beats Continue
// beats Review beats Now when multiple signals fire, because Knowledge
// Loop's distinguishing surface is "what changed since last time" rather
// than fresh-from-stream.
//
// When no v2 evidence is present the function falls back to the v1
// event-type mapping so the planner can be enabled per-entry without an
// all-or-nothing flag day. This means a row with surface_planner_version=2
// is allowed to inherit a v1-shaped placement when no v2 inputs apply.
func decideBucketV2(in SurfaceScoreInputs) sovereignv1.SurfaceBucket {
	// Changed: versioned supersede or a contradiction count is the single
	// strongest signal the user's mental model needs updating. Even one
	// drift outranks fresh observation because the user has already seen
	// the article once. ContradictionCount joins VersionDriftCount here
	// because both come from the same SummarySuperseded chain — keeping
	// them additive avoids accidental "drift but no contradiction"
	// double-counting bugs.
	//
	// CHANGED also outranks ActOutcomeSignal demotion (ADR-000908 §Δ1) —
	// even when the user has shown no engagement, fresh version drift
	// means their mental model is out of date and the system must surface
	// the change.
	if in.VersionDriftCount > 0 || in.ContradictionCount > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	}

	// ADR-000908 §Δ1 demotion: a cumulative outcome signal of ≤ -2 in the
	// v2 score window means the user has actively skipped or stale-saved
	// this entry more than they have engaged with it. Re-promoting it to
	// Now/Continue would repeat a mistake the event log already records;
	// route it into Review for deliberate re-evaluation instead. The
	// threshold of -2 is the smallest single signal that should fire
	// demotion (one no_engagement) — milder negatives (a single
	// stale_save = -1) remain bucket-neutral so a partial signal cannot
	// flap an entry between Now and Review.
	if in.ActOutcomeSignal <= -2 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}
	// ADR-000913 §D-10 demotion: SPECULATION-grade confidence from the
	// recap persist stage means the cluster the entry belongs to has too
	// few evidence anchors to be promoted. Route to Review so the user can
	// re-evaluate when more evidence accumulates. Same priority as the
	// negative ActOutcomeSignal demotion — CHANGED still wins.
	if in.ConfidenceLadder == int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION) {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}

	// Continue: an unfinished Augur thread, an explicit open interaction,
	// a non-zero question-continuation score, or a recent semantic continue
	// action (Phase 2: open / ask / revisit / open-recap with
	// continue_flag=true) puts the entry mid-flow.
	if in.HasAugurLink || in.HasOpenInteraction || in.QuestionContinuationScore > 0 || in.RecentContinueActionCount > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	}

	// Now: strong topic affinity (recap overlap), trending tags, or hot-
	// cluster momentum promote the entry to the foreground. The threshold
	// of 1 is intentional — even one overlap means the article connects to
	// something the user is currently thinking about.
	if in.TopicOverlapCount > 0 || in.TagOverlapCount > 0 || in.RecapClusterMomentum > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	}

	// Review: ADR-000907 §Δ8 reframes Review as the epistemic-change-driven
	// re-evaluation queue. The only entry path is StalenessScore ≥ 2.
	// Contradiction / VersionDrift route to Changed earlier, and
	// HasAugurLink (unfinished thread) routes to Continue, so the v2
	// signals already covered the other Review reasons before we get here.
	//
	// User-driven HomeItemDismissed is intentionally NOT a Review trigger.
	// Dismiss flips dismiss_state/visibility_state to hidden via the entry
	// patch path; placing it in Review would mix "ユーザが捨てた = もう見たくない"
	// with "system が再評価を促す" and break the bucket's semantic.
	//
	// EvidenceDensity is intentionally NOT used as a Review trigger because
	// the NullSurfaceScoreResolver never populates it (always 0), so a
	// "0 == no anchors" rule would route every v1 placement to Review and
	// break the fallback. EvidenceDensity remains in the inputs for
	// downstream diagnostics and a future v2-only resolver can choose to
	// consult it once the v1 fallback path is retired.
	if in.StalenessScore >= 2 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}

	// v1 fallback for events that lack any of the v2 evidence above.
	return v1FallbackBucket(in.EventType)
}

// decideReviewReason picks the epistemic-change driver that put the entry
// into Review (ADR-000907). Pure function of SurfaceScoreInputs — reproject
// reproduces the same reason for the same event log.
//
// Priority follows the canonical contract §6: a version-drift / supersede
// chain is the strongest "your mental model is stale" signal, then a
// contradiction (different supersede semantics), then an unfinished thread,
// then time-decay staleness. Returns NONE when no driver fired so callers
// can distinguish "review bucket but no driver" (a v1 fallback corner case)
// from "review bucket because of X".
func decideReviewReason(in SurfaceScoreInputs) sovereignv1.ReviewReason {
	if in.VersionDriftCount > 0 {
		return sovereignv1.ReviewReason_REVIEW_REASON_VERSION_DRIFT
	}
	if in.ContradictionCount > 0 {
		return sovereignv1.ReviewReason_REVIEW_REASON_CONTRADICTION
	}
	if in.HasAugurLink || in.QuestionContinuationScore > 0 {
		return sovereignv1.ReviewReason_REVIEW_REASON_UNFINISHED_THREAD
	}
	if in.StalenessScore >= 2 {
		return sovereignv1.ReviewReason_REVIEW_REASON_STALENESS
	}
	return sovereignv1.ReviewReason_REVIEW_REASON_NONE
}

func v1FallbackBucket(eventType string) sovereignv1.SurfaceBucket {
	// ADR-000907 §Δ8: HomeItemDismissed no longer maps to Review here. It
	// becomes a no-op on placement (visibility/dismiss state hides the row
	// from the read path), and the fallback bucket value is symbolic. We
	// default to CONTINUE because the entry has prior context — if visibility
	// later flips back, Continue is a more honest landing than Review.
	switch eventType {
	case EventSummaryVersionCreated, EventHomeItemsSeen, EventHomeItemAsked:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	case EventHomeItemOpened, EventHomeItemDismissed:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	case EventHomeItemSuperseded, EventSummarySuperseded:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	default:
		// Unknown / empty event types: prefer Continue over Review so the
		// Review surface stays a deliberate re-evaluation queue rather than
		// a catch-all leftover bucket.
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	}
}
