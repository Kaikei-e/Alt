package knowledge_home_projector

import (
	"time"

	"github.com/google/uuid"
)

// ── wire-capture types ──
//
// These mirror the exact json.RawMessage unmarshal targets already living in
// sovereign_db.Repository's mutation methods (UpsertKnowledgeHomeItem,
// DismissKnowledgeHomeItem, ClearSupersedeState, UpsertTodayDigest,
// UpsertRecallCandidate, PatchKnowledgeHomeItemURL — see
// knowledge-sovereign/app/driver/sovereign_db/repository.go and
// patch_knowledge_home_item_url.go). The projector must produce payloads
// that unmarshal correctly against these targets, because Repository is
// satisfied directly by *sovereign_db.Repository (no intermediate gateway —
// same shape as knowledge_trail_projector). Using distinct "captured*" names
// here (rather than the natural production names like
// "articleCreatedPayload") avoids colliding with the fold implementation's
// own types once projector.go grows the real logic in GREEN.

type capturedHomeItem struct {
	UserID         uuid.UUID  `json:"user_id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	ItemKey        string     `json:"item_key"`
	ItemType       string     `json:"item_type"`
	PrimaryRefID   *uuid.UUID `json:"primary_ref_id"`
	Title          string     `json:"title"`
	SummaryExcerpt string     `json:"summary_excerpt"`
	Tags           []string   `json:"tags"`
	WhyReasons     []struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	} `json:"why_reasons"`
	Score             float64    `json:"score"`
	ScoreOp           string     `json:"score_op"`
	FreshnessAt       *time.Time `json:"freshness_at"`
	PublishedAt       *time.Time `json:"published_at"`
	LastInteractedAt  *time.Time `json:"last_interacted_at"`
	GeneratedAt       time.Time  `json:"generated_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DismissedAt       *time.Time `json:"dismissed_at"`
	ProjectionVersion int        `json:"projection_version"`
	SummaryState      string     `json:"summary_state"`
	SupersedeState    string     `json:"supersede_state"`
	SupersededAt      *time.Time `json:"superseded_at"`
	PreviousRefJSON   string     `json:"previous_ref_json"`
	URL               string     `json:"url"`
}

type capturedDismiss struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	DismissedAt       string `json:"dismissed_at"`
}

type capturedClearSupersede struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
}

type capturedDigest struct {
	UserID               uuid.UUID `json:"user_id"`
	DigestDate           string    `json:"digest_date"`
	NewArticles          int       `json:"new_articles"`
	SummarizedArticles   int       `json:"summarized_articles"`
	UnsummarizedArticles int       `json:"unsummarized_articles"`
	TopTags              []string  `json:"top_tags"`
	UpdatedAt            time.Time `json:"updated_at"`
	// LastEventSeq is a pointer for the same reason the driver's unmarshal
	// target is: a payload that omits the key must be distinguishable from
	// one that sends an explicit 0, because the driver rejects both and the
	// fake has to reject them the same way.
	LastEventSeq *int64 `json:"last_event_seq"`
}

type capturedRecallCandidate struct {
	UserID  uuid.UUID `json:"user_id"`
	ItemKey string    `json:"item_key"`
	Reasons []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"reasons"`
	RecallScore       float64    `json:"recall_score"`
	NextSuggestAt     *time.Time `json:"next_suggest_at"`
	FirstEligibleAt   *time.Time `json:"first_eligible_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ProjectionVersion int        `json:"projection_version"`
}

type capturedURLPatch struct {
	UserID            string `json:"user_id"`
	ItemKey           string `json:"item_key"`
	ProjectionVersion int    `json:"projection_version"`
	URL               string `json:"url"`
}

type capturedSnooze struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	Until      string `json:"until"`
	OccurredAt string `json:"occurred_at"`
}

type capturedRecallDismiss struct {
	UserID     string `json:"user_id"`
	ItemKey    string `json:"item_key"`
	OccurredAt string `json:"occurred_at"`
}
