package knowledge_loop_projector

import (
	"context"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// SurfaceScoreResolver supplies the cross-source evidence required by
// decideBucketV2 to pick a v2 bucket placement. Implementations MUST honor
// the reproject-safe contract:
//
//   - Read only versioned artifacts (`*_versions` tables) and event payload
//     content. Never query mutable views or "latest" state.
//   - Bind every cross-source query by `user_id` (and `article_id` where
//     applicable). F-001 mitigation lives at this seam.
//   - Window any time-bounded counts (e.g. "tags trending in the last 7d")
//     to `event.occurred_at`, never to wall-clock `time.Now()`.
//
// Implementations should return an empty SurfaceScoreInputs (just EventType
// + FreshnessAt populated) when no cross-source evidence applies. The
// projector then lets decideBucketV2 fall back to the v1 mapping, which
// preserves current behavior bit-for-bit.
//
// The default Projector wires NullSurfaceScoreResolver, so the v2 logic is
// reachable from tests and metrics, but the live system continues to write
// surface_planner_version=1 placements until a real resolver is plugged
// in. That keeps cross-service coupling out of this package's blast radius
// while leaving a single drop-in point for the Wave 4 implementation.
type SurfaceScoreResolver interface {
	Resolve(ctx context.Context, ev *sovereign_db.KnowledgeEvent) SurfaceScoreInputs
}

// NullSurfaceScoreResolver is the safe default. It never queries any table
// and always returns empty inputs that decideBucketV2 will resolve to the
// v1 fallback bucket for the event's type.
type NullSurfaceScoreResolver struct{}

// Resolve implements SurfaceScoreResolver and is intentionally a one-line
// shim: the EventType + FreshnessAt are the bare minimum decideBucketV2
// needs to compute its v1 fallback. No cross-source calls.
func (NullSurfaceScoreResolver) Resolve(_ context.Context, ev *sovereign_db.KnowledgeEvent) SurfaceScoreInputs {
	return SurfaceScoreInputs{
		FreshnessAt: ev.OccurredAt,
		EventType:   ev.EventType,
	}
}

// resolveBucket runs the configured resolver for the event and returns the
// bucket decideBucketV2 picks. It also observes the assignment counter so
// dashboards see when v1 fallback dominates vs when v2 evidence kicks in.
//
// The Wave 2 wiring threshold for "this was a v2 placement" is a non-zero
// signal in any of TopicOverlapCount / TagOverlapCount / VersionDriftCount
// / HasAugurLink / HasOpenInteraction. With NullSurfaceScoreResolver every
// placement is therefore tagged "v1" — once a real resolver populates a
// signal, the same code path tags "v2" without further changes.
func (p *Projector) resolveBucket(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
) sovereignv1.SurfaceBucket {
	bucket, _ := p.resolveBucketAndInputs(ctx, ev)
	return bucket
}

// resolveBucketAndInputs returns both the bucket and the resolved inputs so
// downstream code (Wave 4-C narrative override) can re-apply the same v2
// evidence to the WhyPayload without invoking the resolver twice. Callers
// that only need the bucket should keep using resolveBucket.
func (p *Projector) resolveBucketAndInputs(
	ctx context.Context,
	ev *sovereign_db.KnowledgeEvent,
) (sovereignv1.SurfaceBucket, SurfaceScoreInputs) {
	resolver := p.scoreResolver
	if resolver == nil {
		resolver = NullSurfaceScoreResolver{}
	}
	in := resolver.Resolve(ctx, ev)
	bucket := decideBucketV2(in)
	version := "v1"
	if hasV2Signal(in) {
		version = "v2"
	}
	observeSurfaceBucketAssigned(version, bucketMetricLabel(bucket))
	return bucket, in
}

func hasV2Signal(in SurfaceScoreInputs) bool {
	return in.TopicOverlapCount > 0 ||
		in.TagOverlapCount > 0 ||
		in.VersionDriftCount > 0 ||
		in.HasAugurLink ||
		in.HasOpenInteraction
}
