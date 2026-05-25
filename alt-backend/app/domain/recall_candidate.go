package domain

import (
	"time"

	"github.com/google/uuid"
)

// Recall reason code constants.
const (
	ReasonOpenedNotRevisited     = "opened_before_but_not_revisited"
	ReasonRelatedToRecentSearch  = "related_to_recent_search"
	ReasonRelatedToAugurQ        = "related_to_recent_augur_question"
	ReasonRecapContextUnfinished = "recap_context_unfinished"
	ReasonPulseFollowupNeeded    = "pulse_followup_needed"
	ReasonTagInterestOverlap     = "tag_interest_overlap"
	ReasonTagInteraction         = "tag_interaction"
)

// RecallReason explains why an item is being recalled.
type RecallReason struct {
	Type          string `json:"type"`
	Description   string `json:"description"`
	SourceItemKey string `json:"source_item_key,omitempty"`
}

// RecallCandidate represents a candidate for the recall rail.
type RecallCandidate struct {
	UserID            uuid.UUID          `json:"user_id" db:"user_id"`
	ItemKey           string             `json:"item_key" db:"item_key"`
	RecallScore       float64            `json:"recall_score" db:"recall_score"`
	Reasons           []RecallReason     `json:"reasons"`
	NextSuggestAt     *time.Time         `json:"next_suggest_at" db:"next_suggest_at"`
	FirstEligibleAt   *time.Time         `json:"first_eligible_at" db:"first_eligible_at"`
	SnoozedUntil      *time.Time         `json:"snoozed_until" db:"snoozed_until"`
	UpdatedAt         time.Time          `json:"updated_at" db:"updated_at"`
	ProjectionVersion int                `json:"projection_version" db:"projection_version"`
	Item              *KnowledgeHomeItem `json:"item,omitempty"`

	// ADR-000913 §D-9 explainable scoring. WeightSetVersion pins the
	// weights map used; ScoreBreakdown explains each contribution. Empty
	// for legacy rows so the addition is backward compatible.
	WeightSetVersion string                    `json:"weight_set_version,omitempty" db:"weight_set_version"`
	ScoreBreakdown   []RecallScoreContribution `json:"score_breakdown,omitempty"`
}

// RecallWeightSetVersion identifies which weights map produced a candidate.
const (
	RecallWeightSetV1Fixed       = "v1_fixed"
	RecallWeightSetV2HeavyRanker = "v2_heavy_ranker"
)

// RecallScoreContribution is one row of the recall score breakdown — a
// signal code, the weight applied, the contribution (signal_value * weight)
// that landed in the final score, and an explicit negative flag so the UI
// can colour-cue dampening signals separately.
type RecallScoreContribution struct {
	SignalCode   string  `json:"signal_code"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	IsNegative   bool    `json:"is_negative"`
}
