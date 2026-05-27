package knowledge_loop_projector

import (
	"strings"
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// TestAppendRef_PreservesExplicitKind asserts that appendRef forwards the
// kind discriminator the caller provided, untouched. The Augur citation rail
// and the Loop tile UI route on Kind, so SUMMARY / ARTICLE refs must reach
// the wire with their explicit enum value rather than being inferred from a
// string label (the pre-2026-05-27 design that conflated the two concepts).
func TestAppendRef_PreservesExplicitKind(t *testing.T) {
	refs := appendRef(nil,
		evidenceCandidate{RefID: "sv-1", Kind: sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		evidenceCandidate{RefID: "art-1", Label: "Article Title", Kind: sovereignv1.EvidenceKind_EVIDENCE_KIND_ARTICLE},
		evidenceCandidate{RefID: "tag-1", Kind: sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED},
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
	if refs[1].Label != "Article Title" {
		t.Fatalf("article ref Label = %q, want %q", refs[1].Label, "Article Title")
	}
	if refs[2].Kind != sovereignv1.EvidenceKind_EVIDENCE_KIND_UNSPECIFIED {
		t.Fatalf("tags ref Kind = %v (expected UNSPECIFIED)", refs[2].Kind)
	}
}

// TestAppendRef_SkipsEmptyRefID confirms the helper drops zero-RefID entries
// so callers can splat optional payload fields without guarding each one.
func TestAppendRef_SkipsEmptyRefID(t *testing.T) {
	refs := appendRef(nil,
		evidenceCandidate{RefID: "", Kind: sovereignv1.EvidenceKind_EVIDENCE_KIND_SUMMARY},
		evidenceCandidate{RefID: "art-1", Kind: sovereignv1.EvidenceKind_EVIDENCE_KIND_ARTICLE},
	)
	if len(refs) != 1 {
		t.Fatalf("expected empty RefID to be skipped, got %d refs", len(refs))
	}
	if refs[0].RefId != "art-1" {
		t.Fatalf("kept the wrong ref: %q", refs[0].RefId)
	}
}

// TestSanitizeLabel_TruncatesAndStripsMarkup confirms the Label sanitizer
// honours maxLabelBytes and removes the angle-bracket spans the projector
// already rejects in why_text.
func TestSanitizeLabel_TruncatesAndStripsMarkup(t *testing.T) {
	out := sanitizeLabel("<script>alert(1)</script>This is a fine title that goes well beyond the eighty byte budget for an evidence label")
	if len(out) > maxLabelBytes {
		t.Fatalf("expected len <= %d, got %d (%q)", maxLabelBytes, len(out), out)
	}
	if out == "" {
		t.Fatalf("expected non-empty sanitized label")
	}
	// The shared sanitizer drops the angle-bracketed tag spans but allows the
	// text between them to survive (matches the why_text policy: plain text,
	// no Markdown/HTML). The crucial property is that no tag opener leaks
	// through verbatim — that's what enables a future XSS regression check.
	if strings.Contains(out, "<script") {
		t.Fatalf("sanitizer leaked tag opener: %q", out)
	}
}
