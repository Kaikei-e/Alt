package knowledge_loop_projector

import (
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// TestLabelToEvidenceKind pins how the projector tags the discriminator on
// each evidence ref it emits. The downstream Augur citation rail can only
// route correctly if SUMMARY / ARTICLE labels are surfaced; everything else
// must land on UNSPECIFIED so the UI falls back to a non-clickable render
// rather than the bare-UUID-as-relative-href bug.
//
// Pure mapping — reproject-safe by construction (no time, no state).
func TestLabelToEvidenceKind(t *testing.T) {
	cases := []struct {
		label string
		want  sovereignv1.EvidenceKind
	}{
		{"summary", sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		{"previous_summary", sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		{"new_summary", sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		{"what_changed", sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		{"article", sovereignv1.EvidenceKind_EVIDENCE_KIND_ARTICLE},
		{"tags", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"conversation", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"open_event", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"dismiss_event", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"previous_entry", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"new_entry", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"unknown_label", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
		{"", sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := labelToEvidenceKind(tc.label)
			if got != tc.want {
				t.Fatalf("labelToEvidenceKind(%q) = %v, want %v", tc.label, got, tc.want)
			}
		})
	}
}

// TestAppendRefIfPresent_TagsKindFromLabel verifies that the helper used by
// every enricher does not leave Kind = UNSPECIFIED on labels that are
// recognisable as articles or summaries — those are exactly the citations
// the Augur rail must route, and the upstream bug was caused by ARTICLE /
// SUMMARY refs being indistinguishable from WEB URLs.
func TestAppendRefIfPresent_TagsKindFromLabel(t *testing.T) {
	refs := appendRefIfPresent(nil,
		"sv-1", "summary",
		"art-1", "article",
		"tag-1", "tags",
	)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
	if refs[0].Kind != sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY {
		t.Fatalf("summary ref Kind = %v", refs[0].Kind)
	}
	if refs[1].Kind != sovereignv1.EvidenceKind_EVIDENCE_KIND_ARTICLE {
		t.Fatalf("article ref Kind = %v", refs[1].Kind)
	}
	if refs[2].Kind != sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED {
		t.Fatalf("tags ref Kind = %v (expected UNSPECIFIED)", refs[2].Kind)
	}
}
