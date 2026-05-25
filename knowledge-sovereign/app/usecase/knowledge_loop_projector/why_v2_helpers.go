package knowledge_loop_projector

import (
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// WhyPayload v2 producer helpers (ADR-000908 §Δ4).
//
// These are pure functions consumed by EnrichWhyFromEvent and
// OverrideWhyFromSurfaceInputs. They turn the existing WhyKind decision into
// the v2 fields — counter_evidence_refs, confidence_ladder, and
// what_would_change_my_mind — so the projector can populate them
// deterministically on every replay.
//
// The ConfidenceLadder type is defined locally so the helpers compile and are
// testable before the sovereign proto is regenerated with the v2 fields. The
// proto + DB persistence layer maps this local enum to the wire enum in a
// follow-up step; until then, the helpers are exercised in isolation.

// maxCounterEvidenceRefs matches the canonical contract §11 cap (length <= 4).
const maxCounterEvidenceRefs = 4

// maxWhatWouldChangeBytes matches the canonical contract §11 cap (1..256 chars).
const maxWhatWouldChangeBytes = 256

// ConfidenceLadder is the qualitative tier the projector assigns to a WhyKind.
// Values mirror the wire enum alt.knowledge.loop.v1.ConfidenceLadder so the
// later proto-mapping step is a 1:1 translation.
type ConfidenceLadder int

const (
	ConfidenceLadderUnspecified ConfidenceLadder = 0
	ConfidenceLadderSpeculation ConfidenceLadder = 1
	ConfidenceLadderPattern     ConfidenceLadder = 2
	ConfidenceLadderEvidence    ConfidenceLadder = 3
	ConfidenceLadderVerified    ConfidenceLadder = 4
)

// boundCounterEvidence caps the slice to the contract limit. Nil input is
// passed through so callers can chain helpers without nil guards.
func boundCounterEvidence(refs []*sovereignv1.KnowledgeLoopEvidenceRef) []*sovereignv1.KnowledgeLoopEvidenceRef {
	if refs == nil {
		return nil
	}
	if len(refs) > maxCounterEvidenceRefs {
		return refs[:maxCounterEvidenceRefs]
	}
	return refs
}

// confidenceLadderFromKind maps a WhyKind to a coarse confidence tier. The
// mapping anchors §11: CHANGE is evidence-grade because it pins a versioned
// artifact, multi-signal alignments are PATTERN, residual single-signal kinds
// are SPECULATION. VERIFIED is reserved for tiers driven by act_outcome — the
// caller upgrades the result once acted_outcome signal aggregation lands.
func confidenceLadderFromKind(kind sovereignv1.WhyKind) ConfidenceLadder {
	switch kind {
	case sovereignv1.WhyKind_WHY_KIND_CHANGE:
		return ConfidenceLadderEvidence
	case sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE,
		sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY,
		sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING:
		return ConfidenceLadderPattern
	default:
		return ConfidenceLadderSpeculation
	}
}

// whatWouldChangeFromKind returns a short, plain-text falsifier phrase that
// articulates the observation that would invalidate the Why narrative. The
// projector ships this so the UI can render "what would change my mind?"
// inline with the Why text. UNSPECIFIED returns "" — no falsifier means no
// claim, and the projector should not invent one.
func whatWouldChangeFromKind(kind sovereignv1.WhyKind) string {
	switch kind {
	case sovereignv1.WhyKind_WHY_KIND_CHANGE:
		return "A newer summary version supersedes this one."
	case sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE:
		return "The Augur thread is closed or marked resolved."
	case sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY:
		return "Your recent reading shifts away from this topic."
	case sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING:
		return "The matching tag drops out of your recent tag set versions."
	case sovereignv1.WhyKind_WHY_KIND_RECALL:
		return "You open or dismiss this entry, closing the prior-open loop."
	case sovereignv1.WhyKind_WHY_KIND_SOURCE,
		sovereignv1.WhyKind_WHY_KIND_PATTERN:
		return "A fresher event arrives for the same entry."
	default:
		return ""
	}
}
