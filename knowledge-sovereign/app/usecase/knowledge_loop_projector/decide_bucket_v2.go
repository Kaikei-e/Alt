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
	if in.VersionDriftCount > 0 || in.ContradictionCount > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	}

	// Continue: an unfinished Augur thread, an explicit open interaction,
	// or a non-zero question-continuation score puts the entry mid-flow.
	if in.HasAugurLink || in.HasOpenInteraction || in.QuestionContinuationScore > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	}

	// Now: strong topic affinity (recap overlap), trending tags, or hot-
	// cluster momentum promote the entry to the foreground. The threshold
	// of 1 is intentional — even one overlap means the article connects to
	// something the user is currently thinking about.
	if in.TopicOverlapCount > 0 || in.TagOverlapCount > 0 || in.RecapClusterMomentum > 0 {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	}

	// Review: explicitly elevate the bucket from "leftover" to "needs
	// re-evaluation". An entry with stale evidence (StalenessScore ≥ 2 →
	// older than 7 days) lands here so the Review surface becomes the
	// deliberate re-evaluation queue (fb.md §F goal). v1 dismissals also
	// map here for back-compat.
	//
	// EvidenceDensity is intentionally NOT used as a Review trigger because
	// the NullSurfaceScoreResolver never populates it (always 0), so a
	// "0 == no anchors" rule would route every v1 placement to Review and
	// break the fallback. EvidenceDensity remains in the inputs for
	// downstream diagnostics and a future v2-only resolver can choose to
	// consult it once the v1 fallback path is retired.
	if in.StalenessScore >= 2 || isV1ReviewEvent(in.EventType) {
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}

	// v1 fallback for events that lack any of the v2 evidence above.
	return v1FallbackBucket(in.EventType)
}

func isV1ReviewEvent(eventType string) bool {
	switch eventType {
	case EventHomeItemDismissed:
		return true
	default:
		return false
	}
}

func v1FallbackBucket(eventType string) sovereignv1.SurfaceBucket {
	switch eventType {
	case EventSummaryVersionCreated, EventHomeItemsSeen, EventHomeItemAsked:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW
	case EventHomeItemOpened:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE
	case EventHomeItemSuperseded, EventSummarySuperseded:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED
	case EventHomeItemDismissed:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	default:
		return sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW
	}
}
