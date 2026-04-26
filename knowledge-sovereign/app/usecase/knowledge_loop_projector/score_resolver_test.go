package knowledge_loop_projector

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// TestNullSurfaceScoreResolver_Resolve guards the contract that the default
// resolver never queries anything: it just echoes EventType + OccurredAt
// from the event. decideBucketV2 then falls back to the v1 mapping.
func TestNullSurfaceScoreResolver_Resolve(t *testing.T) {
	t.Parallel()

	now := time.Now()
	uid := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventType:  EventSummaryVersionCreated,
		OccurredAt: now,
		UserID:     &uid,
	}
	got := NullSurfaceScoreResolver{}.Resolve(context.Background(), ev)

	if got.EventType != EventSummaryVersionCreated {
		t.Errorf("EventType = %q; want %q", got.EventType, EventSummaryVersionCreated)
	}
	if !got.FreshnessAt.Equal(now) {
		t.Errorf("FreshnessAt = %v; want %v", got.FreshnessAt, now)
	}
	if got.TopicOverlapCount != 0 || got.TagOverlapCount != 0 ||
		got.VersionDriftCount != 0 || got.HasAugurLink || got.HasOpenInteraction {
		t.Errorf("expected zero v2 signals from null resolver; got %+v", got)
	}
}

// TestProjector_ResolveBucket_NullDefault asserts the default Projector
// configuration produces v1-equivalent buckets for each event type, so
// Wave 2 wiring is functionally a no-op for the live system.
func TestProjector_ResolveBucket_NullDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		eventType string
		want      sovereignv1.SurfaceBucket
	}{
		{EventSummaryVersionCreated, sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW},
		{EventHomeItemsSeen, sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW},
		{EventHomeItemAsked, sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW},
		{EventHomeItemOpened, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE},
		{EventHomeItemDismissed, sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW},
		{EventHomeItemSuperseded, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED},
		{EventSummarySuperseded, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED},
	}

	p := &Projector{scoreResolver: NullSurfaceScoreResolver{}}
	uid := uuid.New()

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			t.Parallel()
			ev := &sovereign_db.KnowledgeEvent{
				EventType:  tc.eventType,
				OccurredAt: time.Now(),
				UserID:     &uid,
			}
			got := p.resolveBucket(context.Background(), ev)
			if got != tc.want {
				t.Errorf("resolveBucket(%s) = %v; want %v", tc.eventType, got, tc.want)
			}
		})
	}
}

// fakeResolver lets a test inject specific v2 evidence to verify the
// projector hooks decideBucketV2 with non-null inputs.
type fakeResolver struct {
	out SurfaceScoreInputs
}

func (f fakeResolver) Resolve(_ context.Context, ev *sovereign_db.KnowledgeEvent) SurfaceScoreInputs {
	out := f.out
	out.EventType = ev.EventType
	out.FreshnessAt = ev.OccurredAt
	return out
}

func TestProjector_ResolveBucket_V2EvidencePromotesToChanged(t *testing.T) {
	t.Parallel()

	uid := uuid.New()
	ev := &sovereign_db.KnowledgeEvent{
		EventType:  EventHomeItemsSeen, // v1 mapping → NOW
		OccurredAt: time.Now(),
		UserID:     &uid,
	}
	p := &Projector{
		scoreResolver: fakeResolver{out: SurfaceScoreInputs{VersionDriftCount: 1}},
	}
	got := p.resolveBucket(context.Background(), ev)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED {
		t.Errorf("v2 drift evidence should promote to CHANGED; got %v", got)
	}
}

func TestProjector_WithScoreResolver_NilIgnored(t *testing.T) {
	t.Parallel()

	p := &Projector{scoreResolver: NullSurfaceScoreResolver{}}
	p.WithScoreResolver(nil)
	if p.scoreResolver == nil {
		t.Error("WithScoreResolver(nil) should leave existing resolver in place")
	}
}
