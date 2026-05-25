package knowledge_loop_projector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
			// ADR-000907 §Δ8: HomeItemDismissed no longer drives a Review
			// placement. visibility_state hides the entry regardless of
			// bucket; the symbolic fallback bucket is Continue.
			name: "no v2 signals on a Dismissed event falls back to Continue (epistemic-driven Review)",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   EventHomeItemDismissed,
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
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
			// ADR-000907 §Δ8: Review is reserved for epistemic re-evaluation
			// queues. Unknown event types prefer Continue as a more honest
			// fallback than a leftover Review placement.
			name: "unknown event type falls back to Continue (epistemic-driven Review)",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   "UnknownEvent",
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
		},
		{
			name: "all-zero signals on empty event type falls back to Continue",
			in: SurfaceScoreInputs{
				FreshnessAt: occurredAt,
				EventType:   "",
			},
			want: sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE,
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

// --- Priority tests for the fb.md §B-2 expanded SurfaceScoreInputs --------

// ContradictionCount mirrors VersionDriftCount and must promote Changed when
// either is set, even if Continue or Now signals also fire.
func TestDecideBucketV2_ContradictionBeatsContinue(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		ContradictionCount: 1,
		HasAugurLink:       true, // would otherwise route to Continue
		EventType:          EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED {
		t.Errorf("got %v; want CHANGED (ContradictionCount must outrank HasAugurLink)", got)
	}
}

// QuestionContinuationScore alone (no HasAugurLink, no HasOpenInteraction)
// must still route to Continue.
func TestDecideBucketV2_QuestionContinuationPromotesContinue(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		QuestionContinuationScore: 2,
		EventType:                 EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE {
		t.Errorf("got %v; want CONTINUE (QuestionContinuationScore should promote)", got)
	}
}

// RecapClusterMomentum alone (no TopicOverlap, no TagOverlap) must promote
// the entry to Now.
func TestDecideBucketV2_RecapClusterMomentumPromotesNow(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		RecapClusterMomentum: 1,
		EventType:            EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
		t.Errorf("got %v; want NOW (RecapClusterMomentum should promote)", got)
	}
}

// StalenessScore ≥ 2 (older than 7 days) elevates Review even when no other
// signals fire, turning Review into a deliberate re-evaluation queue rather
// than a leftover bucket.
func TestDecideBucketV2_StalenessElevatesReview(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		StalenessScore: 3, // aging — 7-30 days old
		EventType:      EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW {
		t.Errorf("got %v; want REVIEW (StalenessScore >= 2 should elevate)", got)
	}
}

// StalenessScore < 2 (fresh / current / recent) must NOT elevate Review.
// Without this guard the v1 fallback for SummaryVersionCreated (Now) would
// be silently overridden.
func TestDecideBucketV2_FreshDoesNotElevateReview(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		StalenessScore: 1, // current
		EventType:      EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
		t.Errorf("got %v; want NOW (Staleness=1 should fall through to v1 fallback)", got)
	}
}

// EvidenceDensity == 0 must NOT trigger Review on its own. The Null
// resolver leaves it zero by default; gating Review on it would route every
// v1 placement to Review and break back-compat.
func TestDecideBucketV2_ZeroEvidenceDensityDoesNotForceReview(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		EvidenceDensity: 0, // default
		EventType:       EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
		t.Errorf("got %v; want NOW (zero EvidenceDensity must not break v1 fallback)", got)
	}
}

// ADR-000908 §Δ1: a strong "no engagement" signal (ActOutcomeSignal ≤ -2)
// demotes Now-eligible entries to Review so the Loop stops re-promoting
// content the user has actively skipped. Mirrors the Plan agent's
// down-weight model: engaged=+1, deep=+2, accepted_change=+1, stale_save=-1,
// no_engagement=-2.
func TestDecideBucketV2_StrongNoEngagementDemotesNowToReview(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		TopicOverlapCount: 5,  // would otherwise route to NOW
		ActOutcomeSignal:  -2, // 1× no_engagement
		EventType:         EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW {
		t.Errorf("got %v; want REVIEW (ActOutcomeSignal=-2 must demote Now)", got)
	}
}

// ADR-000908 §Δ1: a strong "no engagement" signal also demotes Continue.
// Once the user has demonstrated they are not engaging, even Continue
// signals (HasOpenInteraction, HasAugurLink) should not force the entry
// back into the foreground.
func TestDecideBucketV2_StrongNoEngagementDemotesContinueToReview(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		HasOpenInteraction: true, // would otherwise route to CONTINUE
		ActOutcomeSignal:   -2,
		EventType:          EventHomeItemOpened,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW {
		t.Errorf("got %v; want REVIEW (ActOutcomeSignal=-2 must demote Continue)", got)
	}
}

// A mild negative signal (single stale_save / partial no_engagement) is not
// enough to override otherwise-strong Now placement. The threshold must be
// crossed to ≤ -2 before bucket demotion fires.
func TestDecideBucketV2_MildNegativeSignalKeepsNow(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		TopicOverlapCount: 5,
		ActOutcomeSignal:  -1, // 1× stale_save, not enough
		EventType:         EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
		t.Errorf("got %v; want NOW (ActOutcomeSignal=-1 must not demote)", got)
	}
}

// CHANGED must outrank ActOutcomeSignal demotion: even when the user has
// shown no engagement, a fresh version drift means their mental model is
// out of date and the system must surface that — staying in Review would
// silently hide an important update.
func TestDecideBucketV2_ChangedBeatsActOutcomeDemotion(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		VersionDriftCount: 1,
		ActOutcomeSignal:  -4, // even strong no engagement
		EventType:         EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED {
		t.Errorf("got %v; want CHANGED (drift outranks act outcome demotion)", got)
	}
}

// Positive ActOutcomeSignal (engaged / deep_engagement) does not change
// bucket placement. Plan agent: positive is used for within-bucket ranking,
// not bucket selection.
func TestDecideBucketV2_PositiveActOutcomeKeepsBucket(t *testing.T) {
	t.Parallel()
	in := SurfaceScoreInputs{
		TopicOverlapCount: 1,
		ActOutcomeSignal:  3, // deep_engagement
		EventType:         EventSummaryVersionCreated,
	}
	got := decideBucketV2(in)
	if got != sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
		t.Errorf("got %v; want NOW (positive signal does not change bucket)", got)
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

// decideReviewReason (ADR-000907) is the epistemic-change driver for the
// Review bucket. Priority: version_drift > contradiction > unfinished_thread
// > staleness > none. The function is pure; reproject must produce the same
// reason for the same SurfaceScoreInputs.

func TestDecideReviewReason_VersionDriftWins(t *testing.T) {
	t.Parallel()

	got := decideReviewReason(SurfaceScoreInputs{
		VersionDriftCount:  1,
		ContradictionCount: 1,
		HasAugurLink:       true,
		StalenessScore:     4,
	})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_VERSION_DRIFT, got,
		"version_drift must outrank every other review signal")
}

func TestDecideReviewReason_ContradictionOverStaleness(t *testing.T) {
	t.Parallel()

	got := decideReviewReason(SurfaceScoreInputs{
		ContradictionCount: 1,
		StalenessScore:     4,
	})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_CONTRADICTION, got,
		"contradiction must outrank staleness")
}

func TestDecideReviewReason_UnfinishedAugurThread(t *testing.T) {
	t.Parallel()

	got := decideReviewReason(SurfaceScoreInputs{
		HasAugurLink:   true,
		StalenessScore: 4,
	})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_UNFINISHED_THREAD, got)

	gotByCount := decideReviewReason(SurfaceScoreInputs{
		QuestionContinuationScore: 2,
	})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_UNFINISHED_THREAD, gotByCount,
		"non-zero QuestionContinuationScore must also count as an unfinished thread")
}

func TestDecideReviewReason_StalenessOnly(t *testing.T) {
	t.Parallel()

	got := decideReviewReason(SurfaceScoreInputs{StalenessScore: 2})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_STALENESS, got)
}

func TestDecideReviewReason_NoneWhenNoSignal(t *testing.T) {
	t.Parallel()

	got := decideReviewReason(SurfaceScoreInputs{})
	require.Equal(t, sovereignv1.ReviewReason_REVIEW_REASON_NONE, got,
		"empty inputs must yield NONE so the projection column has a deterministic default")
}

// ADR-000913 §D-10 — persist-stage confidence ladder demotion.
// SPECULATION should demote a Now/Continue placement to Review.

func TestDecideBucketV2_SpeculationLadder_DemotesToReview(t *testing.T) {
	t.Parallel()

	got := decideBucketV2(SurfaceScoreInputs{
		TopicOverlapCount: 3,
		EventType:         EventSummaryVersionCreated,
		ConfidenceLadder:  int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION),
	})
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW, got,
		"SPECULATION confidence must demote NOW signals to Review")
}

func TestDecideBucketV2_VerifiedLadder_DoesNotDemoteNow(t *testing.T) {
	t.Parallel()

	got := decideBucketV2(SurfaceScoreInputs{
		TopicOverlapCount: 3,
		EventType:         EventSummaryVersionCreated,
		ConfidenceLadder:  int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_VERIFIED),
	})
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW, got,
		"VERIFIED confidence must not demote NOW")
}

func TestDecideBucketV2_VersionDriftOverridesSpeculation(t *testing.T) {
	t.Parallel()

	got := decideBucketV2(SurfaceScoreInputs{
		VersionDriftCount: 1,
		ConfidenceLadder:  int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION),
	})
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED, got,
		"version drift must still surface as Changed even under SPECULATION confidence")
}

func TestParseSurfaceScoreInputs_ParsesConfidenceLadder(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	in := parseSurfaceScoreInputs(map[string]any{
		"persist_stage_confidence_ladder": "speculation",
	}, occurredAt)
	require.Equal(t, int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_SPECULATION), in.ConfidenceLadder)
}

func TestParseSurfaceScoreInputs_MissingLadder_DefaultsUnspecified(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	in := parseSurfaceScoreInputs(map[string]any{}, occurredAt)
	require.Equal(t, int32(sovereignv1.ConfidenceLadder_CONFIDENCE_LADDER_UNSPECIFIED), in.ConfidenceLadder)
}
