package knowledge_loop_projector

import (
	"testing"
	"time"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestDecideBucketV2(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   SurfaceScoreInputs
		want sovereignv1.SurfaceBucket
	}{
		{
			name: "version drift outranks any other signal — entry goes to Changed",
			in: SurfaceScoreInputs{
				VersionDriftCount:  3,
				HasAugurLink:       true,
				HasOpenInteraction: true,
				TopicOverlapCount:  5,
				TagOverlapCount:    7,
				FreshnessAt:        occurredAt,
				EventType:          EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		},
		{
			name: "single version drift still goes to Changed",
			in: SurfaceScoreInputs{
				VersionDriftCount: 1,
				FreshnessAt:       occurredAt,
				EventType:         EventHomeItemOpened,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		},
		{
			name: "augur link without drift routes to Continue",
			in: SurfaceScoreInputs{
				HasAugurLink:      true,
				TopicOverlapCount: 9,
				FreshnessAt:       occurredAt,
				EventType:         EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		},
		{
			name: "open interaction without drift routes to Continue",
			in: SurfaceScoreInputs{
				HasOpenInteraction: true,
				FreshnessAt:        occurredAt,
				EventType:          EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		},
		{
			name: "topic overlap with no continuity signal lands in Now",
			in: SurfaceScoreInputs{
				TopicOverlapCount: 2,
				FreshnessAt:       occurredAt,
				EventType:         EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		},
		{
			name: "tag overlap alone lands in Now",
			in: SurfaceScoreInputs{
				TagOverlapCount: 1,
				FreshnessAt:     occurredAt,
				EventType:       EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		},
		{
			name: "no v2 signals on a Dismissed event falls back to Review",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemDismissed,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW,
		},
		{
			name: "no v2 signals on SummaryVersionCreated falls back to Now (v1 mapping)",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		},
		{
			name: "no v2 signals on HomeItemOpened falls back to Continue (v1 mapping)",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemOpened,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		},
		{
			name: "no v2 signals on HomeItemSuperseded falls back to Changed",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemSuperseded,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		},
		{
			name: "no v2 signals on SummarySuperseded falls back to Changed",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventSummarySuperseded,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		},
		{
			name: "no v2 signals on HomeItemsSeen falls back to Now",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemsSeen,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		},
		{
			name: "no v2 signals on HomeItemAsked falls back to Now",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemAsked,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
		},
		{
			name: "unknown event type falls back to Review",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   "UnknownEvent",
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW,
		},
		{
			name: "all-zero signals on empty event type falls back to Review",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   "",
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW,
		},
		{
			name: "augur + drift: drift wins (priority order)",
			in: SurfaceScoreInputs{
				VersionDriftCount: 1,
				HasAugurLink:      true,
				FreshnessAt:       occurredAt,
				EventType:         EventHomeItemOpened,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED,
		},
		{
			name: "augur + topic: augur wins (priority order)",
			in: SurfaceScoreInputs{
				HasAugurLink:      true,
				TopicOverlapCount: 4,
				FreshnessAt:       occurredAt,
				EventType:         EventSummaryVersionCreated,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decideBucketV2(tc.in)
			if got != tc.want {
				t.Fatalf("decideBucketV2(%+v) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestDecideBucketV2_Determinism asserts the function is pure: the same
// inputs must always produce the same bucket regardless of when it runs.
// This guards reproject-safety — replaying the same event must yield the
// same surface placement on a fresh projection.
func TestDecideBucketV2_Determinism(t *testing.T) {
	t.Parallel()

	in := SurfaceScoreInputs{
		TopicOverlapCount:  2,
		TagOverlapCount:    1,
		HasAugurLink:       false,
		VersionDriftCount:  0,
		HasOpenInteraction: false,
		FreshnessAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EventType:          EventSummaryVersionCreated,
	}

	want := decideBucketV2(in)
	for i := 0; i < 1000; i++ {
		got := decideBucketV2(in)
		if got != want {
			t.Fatalf("decideBucketV2 not deterministic at iter %d: got %v want %v", i, got, want)
		}
	}
}
