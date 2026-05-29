package knowledge_loop_usecase

import (
	"testing"
	"time"

	"alt/domain"

	"github.com/stretchr/testify/require"
)

func ladder(c domain.ConfidenceLadder) *domain.ConfidenceLadder { return &c }

// buildLensFixtures returns four entries, each strongly matching one lens, in a
// deliberately non-lens order so the sort has to do real work.
func buildLensFixtures() (fresh, research, decide, recall *domain.KnowledgeLoopEntry) {
	fresh = &domain.KnowledgeLoopEntry{
		EntryKey:      "fresh",
		WhyKind:       domain.WhyKindSource,
		SurfaceBucket: domain.SurfaceNow,
		FreshnessAt:   time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC), // newest
	}
	research = &domain.KnowledgeLoopEntry{
		EntryKey:            "research",
		WhyKind:             domain.WhyKindChange,
		SurfaceBucket:       domain.SurfaceChanged,
		WhyEvidenceRefs:     []domain.EvidenceRef{{RefID: "e1"}, {RefID: "e2"}, {RefID: "e3"}},
		WhyConfidenceLadder: ladder(domain.ConfidenceLadderEvidence),
		FreshnessAt:         time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
	}
	decide = &domain.KnowledgeLoopEntry{
		EntryKey:        "decide",
		WhyKind:         domain.WhyKindPattern,
		SurfaceBucket:   domain.SurfaceNow,
		ProposedStage:   domain.LoopStageDecide,
		DecisionOptions: []byte(`[{"intent":"open"},{"intent":"compare"},{"intent":"ask"}]`),
		FreshnessAt:     time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
	}
	recall = &domain.KnowledgeLoopEntry{
		EntryKey:      "recall",
		WhyKind:       domain.WhyKindRecall,
		SurfaceBucket: domain.SurfaceReview,
		ReviewReason:  domain.ReviewReasonStaleness,
		FreshnessAt:   time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
	}
	return
}

func TestApplyLensWeighting_EachLensFavoursItsEntry(t *testing.T) {
	cases := []struct {
		lens string
		want string
	}{
		{"browse", "fresh"},
		{"research", "research"},
		{"decide", "decide"},
		{"recall", "recall"},
	}
	for _, tc := range cases {
		t.Run(tc.lens, func(t *testing.T) {
			fresh, research, decide, recall := buildLensFixtures()
			// Input order intentionally does NOT match any lens.
			entries := []*domain.KnowledgeLoopEntry{recall, fresh, research, decide}
			applyLensWeighting(entries, tc.lens)
			require.Equalf(t, tc.want, entries[0].EntryKey,
				"lens %q must surface the %q entry first", tc.lens, tc.want)
		})
	}
}

func TestApplyLensWeighting_DefaultPreservesProjectionOrder(t *testing.T) {
	fresh, research, decide, recall := buildLensFixtures()
	entries := []*domain.KnowledgeLoopEntry{recall, fresh, research, decide}
	applyLensWeighting(entries, "default")
	// default leaves the read query's order untouched (stable sort, score 0).
	got := []string{entries[0].EntryKey, entries[1].EntryKey, entries[2].EntryKey, entries[3].EntryKey}
	require.Equal(t, []string{"recall", "fresh", "research", "decide"}, got)
}

func TestApplyLensWeighting_NoPanicOnEmptyOrSingle(t *testing.T) {
	applyLensWeighting(nil, "research")
	applyLensWeighting([]*domain.KnowledgeLoopEntry{{EntryKey: "solo"}}, "recall")
}
