package domain

import (
	"time"

	"github.com/google/uuid"
)

// Signal type constants for recall signals.
const (
	SignalOpened             = "opened"
	SignalSearchRelated      = "search_related"
	SignalAugurReferenced    = "augur_referenced"
	SignalRecapContextUnread = "recap_context_unread"
	SignalPulseFollowup      = "pulse_followup"
	SignalTagInterest        = "tag_interest"
	SignalTagClicked         = "tag_clicked"

	// ADR-000913 §D-9 negative recall signals. Heavy-Ranker grounding from
	// Twitter's the-algorithm-ml introduces explicit negative weights so
	// content the user has already pushed away does not keep cycling back
	// into the recall rail. Producer wiring is tracked separately; the
	// scorer treats absence as 0 contribution.
	SignalRecentlyDismissed    = "recently_dismissed"
	SignalLowSummaryConfidence = "low_summary_confidence"
)

// RecallSignal represents a user interaction signal that feeds the recall scoring algorithm.
type RecallSignal struct {
	SignalID       uuid.UUID      `json:"signal_id" db:"signal_id"`
	UserID         uuid.UUID      `json:"user_id" db:"user_id"`
	ItemKey        string         `json:"item_key" db:"item_key"`
	SignalType     string         `json:"signal_type" db:"signal_type"`
	SignalStrength float64        `json:"signal_strength" db:"signal_strength"`
	OccurredAt     time.Time      `json:"occurred_at" db:"occurred_at"`
	Payload        map[string]any `json:"payload" db:"payload"`
}
