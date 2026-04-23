package knowledge_loop_usecase

import (
	"alt/domain"
	"fmt"
)

// whyCodeToKind is the exhaustive mapping from Phase-0 Knowledge Home why codes to
// the Knowledge Loop WhyKind taxonomy. Keep this table in lockstep with
// docs/plan/knowledge-loop-canonical-contract.md §11. A change here MUST bump
// WhyMappingVersion (in validator.go) and trigger a full reproject via runbook.
var whyCodeToKind = map[string]domain.WhyKind{
	domain.WhyNewUnread:             domain.WhyKindSource,
	domain.WhySummaryCompleted:      domain.WhyKindSource,
	domain.WhyTagHotspot:            domain.WhyKindPattern,
	domain.WhyRecentInterestMatch:   domain.WhyKindPattern,
	domain.WhyRelatedToRecentSearch: domain.WhyKindPattern,
	domain.WhyInWeeklyRecap:         domain.WhyKindRecall,
	domain.WhyPulseNeedToKnow:       domain.WhyKindChange,
}

// MapPhase0WhyCodeToKind returns the canonical WhyKind for a Phase-0 why code.
// An unknown code is a hard error: the taxonomy must stay exhaustive.
func MapPhase0WhyCodeToKind(code string) (domain.WhyKind, error) {
	k, ok := whyCodeToKind[code]
	if !ok {
		return "", fmt.Errorf("%w: unknown why code %q (update why_mapper and bump WhyMappingVersion)", ErrInvalidArgument, code)
	}
	return k, nil
}

// AllPhase0WhyCodes returns all supported Phase-0 codes. Useful for exhaustiveness tests.
func AllPhase0WhyCodes() []string {
	out := make([]string, 0, len(whyCodeToKind))
	for k := range whyCodeToKind {
		out = append(out, k)
	}
	return out
}
