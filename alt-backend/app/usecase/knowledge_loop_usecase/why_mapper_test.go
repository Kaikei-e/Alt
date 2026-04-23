package knowledge_loop_usecase

import (
	"alt/domain"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPhase0WhyCodes_AllMapped exhaustively maps every canonical Phase-0 why code to a WhyKind.
// If a new code is introduced to domain/knowledge_home_item.go without updating this mapper,
// the test flags it so WhyMappingVersion can be bumped and a reproject scheduled.
func TestPhase0WhyCodes_AllMapped(t *testing.T) {
	canonical := []string{
		domain.WhyNewUnread,
		domain.WhyInWeeklyRecap,
		domain.WhyPulseNeedToKnow,
		domain.WhyTagHotspot,
		domain.WhyRecentInterestMatch,
		domain.WhyRelatedToRecentSearch,
		domain.WhySummaryCompleted,
	}
	for _, code := range canonical {
		kind, err := MapPhase0WhyCodeToKind(code)
		require.NoError(t, err, "code %q must be mapped; update why_mapper.go and bump WhyMappingVersion", code)
		require.NotEmpty(t, kind)
	}
}

func TestPhase0WhyCodes_UnknownIsRejected(t *testing.T) {
	_, err := MapPhase0WhyCodeToKind("not_a_real_code")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidArgument)
}
