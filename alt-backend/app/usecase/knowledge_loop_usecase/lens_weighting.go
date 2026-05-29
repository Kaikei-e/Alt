package knowledge_loop_usecase

import (
	"encoding/json"
	"sort"

	"alt/domain"
)

// canonicalLensModeID is the single partition the projector writes Loop entries
// into. The lens the user selects (research / browse / decide / recall) is a
// *view* over these shared entries — a Surface-Planner-style weighting input
// (ADR-000909 §Δ5) — never a separate storage partition. Reads therefore always
// target this partition and re-rank by the requested lens (applyLensWeighting),
// so switching lenses re-orders the same canonical entries instead of querying
// an empty partition. This is what re-grounds the lens model after the
// 2026-05-29 incident where every non-"default" lens returned an empty page.
const canonicalLensModeID = "default"

// applyLensWeighting re-orders entries in place by a lens-specific score so the
// same canonical entries surface differently per cognitive mode. It is a pure
// function over already-projected signals (no IO, reproject-independent) and
// uses a stable sort, so entries the lens does not distinguish keep their
// projection order (loop_priority → render_depth → freshness, set by the read
// query). Unknown / browse / default lenses leave the projection order intact.
func applyLensWeighting(entries []*domain.KnowledgeLoopEntry, lensModeID string) {
	if len(entries) < 2 {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return lensScore(entries[i], lensModeID) > lensScore(entries[j], lensModeID)
	})
}

func lensScore(e *domain.KnowledgeLoopEntry, lens string) float64 {
	switch lens {
	case "research":
		return researchScore(e)
	case "decide":
		return decideScore(e)
	case "recall":
		return recallScore(e)
	case "browse":
		// Browse is recency-led: most recently fresh entries float up.
		return float64(e.FreshnessAt.Unix())
	default:
		// default / unknown: preserve the read query's projection order.
		return 0
	}
}

// researchScore favours evidence depth, calibrated confidence, and change —
// the signals a "what's the state of my understanding" mode wants up front.
func researchScore(e *domain.KnowledgeLoopEntry) float64 {
	s := float64(len(e.WhyEvidenceRefs))*2.0 + float64(len(e.WhyCounterEvidenceRefs))*1.5
	if e.WhyConfidenceLadder != nil {
		s += confidenceLadderRank(*e.WhyConfidenceLadder)
	}
	if e.WhyKind == domain.WhyKindChange {
		s += 3
	}
	if e.SurfaceBucket == domain.SurfaceChanged {
		s += 2
	}
	return s
}

// decideScore favours decision-readiness: entries that present options and sit
// at (or near) the Decide stage.
func decideScore(e *domain.KnowledgeLoopEntry) float64 {
	s := float64(decisionOptionCount(e.DecisionOptions))
	if e.ProposedStage == domain.LoopStageDecide {
		s += 3
	}
	if e.WhyKind == domain.WhyKindTopicAffinity || e.WhyKind == domain.WhyKindPattern {
		s += 1.5
	}
	return s
}

// recallScore favours re-surfacing: recall why-codes, epistemic-change review
// reasons, unfinished threads, and the Review bucket.
func recallScore(e *domain.KnowledgeLoopEntry) float64 {
	s := 0.0
	switch e.WhyKind {
	case domain.WhyKindRecall:
		s += 3
	case domain.WhyKindUnfinishedContinue:
		s += 2
	}
	if e.ReviewReason != "" && e.ReviewReason != domain.ReviewReasonNone {
		s += 2
	}
	if e.SurfaceBucket == domain.SurfaceReview {
		s += 2
	}
	return s
}

func confidenceLadderRank(c domain.ConfidenceLadder) float64 {
	switch c {
	case domain.ConfidenceLadderSpeculation:
		return 1
	case domain.ConfidenceLadderPattern:
		return 2
	case domain.ConfidenceLadderEvidence:
		return 3
	case domain.ConfidenceLadderVerified:
		return 4
	}
	return 0
}

// decisionOptionCount returns the number of decision options encoded in the
// JSONB blob. Malformed / empty blobs count as zero — the decoder tolerates
// stale rows the same way the handler's decoders do.
func decisionOptionCount(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return 0
	}
	return len(raw)
}
