package knowledge_loop_projector

import (
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// These helpers prepare the WhyPayload v2 fields (counter_evidence_refs,
// confidence_ladder, what_would_change_my_mind) defined in ADR-000908 §Δ4.
//
// They are pure: same input always produces the same output. Reproject safety
// depends on this — the projector replays event streams and the v2 fields
// must converge deterministically. The helpers do not call time.Now or read
// projection state.
//
// Wiring into the proto + DB happens in a follow-up step (sovereign proto
// regeneration + migration for `why_counter_evidence_refs`,
// `why_confidence_ladder`, `why_what_would_change_my_mind` columns). This test
// pins the pure behaviour first so the producer contract is locked before the
// transport layer changes.

func TestBoundCounterEvidence_CapsAtFour(t *testing.T) {
	t.Parallel()

	in := []*sovereignv1.KnowledgeLoopEvidenceRef{
		{RefId: "1", Label: "a"},
		{RefId: "2", Label: "b"},
		{RefId: "3", Label: "c"},
		{RefId: "4", Label: "d"},
		{RefId: "5", Label: "e"},
		{RefId: "6", Label: "f"},
	}

	got := boundCounterEvidence(in)
	if len(got) != maxCounterEvidenceRefs {
		t.Fatalf("expected length %d, got %d", maxCounterEvidenceRefs, len(got))
	}
	if got[0].RefId != "1" || got[3].RefId != "4" {
		t.Fatalf("expected first four refs preserved in order, got %+v", got)
	}
}

func TestBoundCounterEvidence_PassThroughBelowCap(t *testing.T) {
	t.Parallel()

	in := []*sovereignv1.KnowledgeLoopEvidenceRef{
		{RefId: "1", Label: "a"},
		{RefId: "2", Label: "b"},
	}

	got := boundCounterEvidence(in)
	if len(got) != 2 {
		t.Fatalf("expected length 2, got %d", len(got))
	}
}

func TestBoundCounterEvidence_NilInput(t *testing.T) {
	t.Parallel()

	if got := boundCounterEvidence(nil); got != nil {
		t.Fatalf("expected nil pass-through, got %+v", got)
	}
}

func TestConfidenceLadderFromKind(t *testing.T) {
	t.Parallel()

	// Mapping rationale (ADR-000908 §Δ4 + canonical contract §11):
	//   CHANGE (supersede) → EVIDENCE: a versioned artifact is the strongest
	//     verifiable signal we can attach.
	//   UNFINISHED_CONTINUE (open augur thread) → PATTERN: explicit prior
	//     intent but not yet artifact-bound.
	//   TOPIC_AFFINITY / TAG_TRENDING → PATTERN: multiple weak signals align.
	//   RECALL / SOURCE / PATTERN / UNSPECIFIED → SPECULATION: residual or
	//     single low-strength evidence.
	tests := []struct {
		name string
		kind sovereignv1.WhyKind
		want ConfidenceLadder
	}{
		{"change is evidence-grade", sovereignv1.WhyKind_WHY_KIND_CHANGE, ConfidenceLadderEvidence},
		{"unfinished continue is pattern", sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE, ConfidenceLadderPattern},
		{"topic affinity is pattern", sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY, ConfidenceLadderPattern},
		{"tag trending is pattern", sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING, ConfidenceLadderPattern},
		{"recall is speculation", sovereignv1.WhyKind_WHY_KIND_RECALL, ConfidenceLadderSpeculation},
		{"source is speculation", sovereignv1.WhyKind_WHY_KIND_SOURCE, ConfidenceLadderSpeculation},
		{"unspecified is speculation", sovereignv1.WhyKind_WHY_KIND_UNSPECIFIED, ConfidenceLadderSpeculation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confidenceLadderFromKind(tt.kind); got != tt.want {
				t.Fatalf("kind=%v want=%v got=%v", tt.kind, tt.want, got)
			}
		})
	}
}

func TestConfidenceLadderFromKind_IsDeterministic(t *testing.T) {
	t.Parallel()

	// Reproject safety: calling twice with the same input must yield the same
	// output (no time, no global state).
	for _, kind := range []sovereignv1.WhyKind{
		sovereignv1.WhyKind_WHY_KIND_CHANGE,
		sovereignv1.WhyKind_WHY_KIND_RECALL,
		sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY,
	} {
		first := confidenceLadderFromKind(kind)
		second := confidenceLadderFromKind(kind)
		if first != second {
			t.Fatalf("non-deterministic for kind=%v: first=%v second=%v", kind, first, second)
		}
	}
}

func TestWhatWouldChangeFromKind_ReturnsConcretePhrase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		kind        sovereignv1.WhyKind
		mustContain string
	}{
		{"change cites newer version invalidation", sovereignv1.WhyKind_WHY_KIND_CHANGE, "newer"},
		{"unfinished_continue cites thread resolution", sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE, "thread"},
		{"topic_affinity cites topic divergence", sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY, "topic"},
		{"tag_trending cites tag drop", sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING, "tag"},
		{"recall cites prior open", sovereignv1.WhyKind_WHY_KIND_RECALL, "open"},
		{"source cites freshness", sovereignv1.WhyKind_WHY_KIND_SOURCE, "fresh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := whatWouldChangeFromKind(tt.kind)
			if got == "" {
				t.Fatalf("expected non-empty phrase for kind=%v", tt.kind)
			}
			if !contains(got, tt.mustContain) {
				t.Fatalf("kind=%v expected phrase to contain %q, got %q", tt.kind, tt.mustContain, got)
			}
		})
	}
}

func TestWhatWouldChangeFromKind_BoundedLength(t *testing.T) {
	t.Parallel()

	// Canonical contract §11: what_would_change_my_mind is 1..256 chars.
	for _, kind := range allWhyKindsForTest() {
		got := whatWouldChangeFromKind(kind)
		if l := len(got); l < 1 || l > maxWhatWouldChangeBytes {
			t.Fatalf("kind=%v phrase length %d outside 1..%d (text=%q)", kind, l, maxWhatWouldChangeBytes, got)
		}
	}
}

func TestWhatWouldChangeFromKind_UnspecifiedReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Unspecified means "no opinion on counter-conditions"; the projector
	// should not invent a falsifiable claim out of nothing.
	if got := whatWouldChangeFromKind(sovereignv1.WhyKind_WHY_KIND_UNSPECIFIED); got != "" {
		t.Fatalf("expected empty for UNSPECIFIED, got %q", got)
	}
}

// --- shared helpers --------------------------------------------------------

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func allWhyKindsForTest() []sovereignv1.WhyKind {
	return []sovereignv1.WhyKind{
		sovereignv1.WhyKind_WHY_KIND_SOURCE,
		sovereignv1.WhyKind_WHY_KIND_PATTERN,
		sovereignv1.WhyKind_WHY_KIND_RECALL,
		sovereignv1.WhyKind_WHY_KIND_CHANGE,
		sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY,
		sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING,
		sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE,
	}
}
