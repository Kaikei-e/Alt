package domain

import (
	"time"

	"github.com/google/uuid"
)

// Recall reason code constants.
const (
	ReasonOpenedNotRevisited    = "opened_before_but_not_revisited"
	ReasonRelatedToRecentSearch = "related_to_recent_search"
	ReasonRelatedToAugurQ       = "related_to_recent_augur_question"
	ReasonRecapContextUnfinished = "recap_context_unfinished"
	ReasonPulseFollowupNeeded   = "pulse_followup_needed"
	ReasonTagInterestOverlap    = "tag_interest_overlap"
	ReasonTagInteraction        = "tag_interaction"
)

// RecallReason explains why an item is being recalled.
type RecallReason struct {
	Type          string `json:"type"`
	Description   string `json:"description"`
	SourceItemKey string `json:"source_item_key,omitempty"`
}

// RecallCandidate represents a candidate for the recall rail.
type RecallCandidate struct {
	UserID            uuid.UUID      `json:"user_id" db:"user_id"`
	ItemKey           string         `json:"item_key" db:"item_key"`
	RecallScore       float64        `json:"recall_score" db:"recall_score"`
	Reasons           []RecallReason `json:"reasons"`
	NextSuggestAt     *time.Time     `json:"next_suggest_at" db:"next_suggest_at"`
	FirstEligibleAt   *time.Time     `json:"first_eligible_at" db:"first_eligible_at"`
	SnoozedUntil      *time.Time     `json:"snoozed_until" db:"snoozed_until"`
	UpdatedAt         time.Time      `json:"updated_at" db:"updated_at"`
	ProjectionVersion int            `json:"projection_version" db:"projection_version"`
	Item              *KnowledgeHomeItem `json:"item,omitempty"`
}
