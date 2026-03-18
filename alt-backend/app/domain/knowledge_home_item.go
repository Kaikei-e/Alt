package domain

import (
	"time"

	"github.com/google/uuid"
)

// Item type constants.
const (
	ItemArticle     = "article"
	ItemRecapAnchor = "recap_anchor"
	ItemPulseAnchor = "pulse_anchor"
)

// Why code constants.
const (
	WhyNewUnread            = "new_unread"
	WhyInWeeklyRecap        = "in_weekly_recap"
	WhyPulseNeedToKnow      = "pulse_need_to_know"
	WhyTagHotspot           = "tag_hotspot"
	WhyRecentInterestMatch  = "recent_interest_match"
	WhyRelatedToRecentSearch = "related_to_recent_search"
	WhySummaryCompleted     = "summary_completed"
)

// WhyReason explains why an item appears in the Knowledge Home.
type WhyReason struct {
	Code  string `json:"code"`
	RefID string `json:"ref_id,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// KnowledgeHomeItem represents a single item in the Knowledge Home feed.
type KnowledgeHomeItem struct {
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ItemKey         string     `json:"item_key" db:"item_key"`
	ItemType        string     `json:"item_type" db:"item_type"`
	PrimaryRefID    *uuid.UUID `json:"primary_ref_id" db:"primary_ref_id"`
	Title           string     `json:"title" db:"title"`
	SummaryExcerpt  string     `json:"summary_excerpt" db:"summary_excerpt"`
	Tags            []string   `json:"tags"`
	WhyReasons      []WhyReason `json:"why_reasons"`
	Score           float64    `json:"score" db:"score"`
	FreshnessAt     *time.Time `json:"freshness_at" db:"freshness_at"`
	PublishedAt     *time.Time `json:"published_at" db:"published_at"`
	LastInteractedAt *time.Time `json:"last_interacted_at" db:"last_interacted_at"`
	GeneratedAt      time.Time  `json:"generated_at" db:"generated_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
	ProjectionVersion int        `json:"projection_version" db:"projection_version"`
	SummaryState      string     `json:"summary_state" db:"summary_state"`
	SupersedeState    string     `json:"supersede_state,omitempty" db:"supersede_state"`
	SupersededAt      *time.Time `json:"superseded_at,omitempty" db:"superseded_at"`
	PreviousRefJSON   string     `json:"previous_ref_json,omitempty" db:"previous_ref_json"`
	Link              string     `json:"link,omitempty" db:"link"`
}

// Summary state constants.
const (
	SummaryStateMissing = "missing"
	SummaryStatePending = "pending"
	SummaryStateReady   = "ready"
)

// Supersede state constants.
const (
	SupersedeSummaryUpdated  = "summary_updated"
	SupersedeTagsUpdated     = "tags_updated"
	SupersedeReasonUpdated   = "reason_updated"
	SupersedeMultipleUpdated = "multiple_updated"
	// Deprecated: use SupersedeMultipleUpdated instead.
	SupersedeBothUpdated = "both_updated"
)
