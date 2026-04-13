package domain

import (
	"time"

	"github.com/google/uuid"
)

// MorningArticleGroup represents a group of similar articles for the morning update.
type MorningArticleGroup struct {
	GroupID   uuid.UUID
	ArticleID uuid.UUID
	IsPrimary bool
	CreatedAt time.Time
	// Article is the joined article data
	Article *Article
}

// MorningUpdate represents the aggregated update for the frontend.
type MorningUpdate struct {
	GroupID        uuid.UUID  `json:"group_id"`
	PrimaryArticle *Article   `json:"primary_article"`
	Duplicates     []*Article `json:"duplicates"`
}

// MorningLetterDocument represents the full Morning Letter for a given date.
type MorningLetterDocument struct {
	ID                 string
	TargetDate         string
	EditionTimezone    string
	IsDegraded         bool
	SchemaVersion      int
	GenerationRevision int
	Model              string
	CreatedAt          time.Time
	Etag               string
	Body               MorningLetterBody
}

// MorningLetterBody is the editorial content of the letter.
type MorningLetterBody struct {
	Lead                  string
	Sections              []MorningLetterSection
	GeneratedAt           time.Time
	SourceRecapWindowDays *int
	// ThroughLine is a one-sentence editorial thread tying the day's
	// events together. Produced deterministically by the projector; the
	// LLM may rewrite for tone but must not change meaning.
	ThroughLine string
	// PreviousLetterRef points at the prior edition in the same timezone.
	// Nil on the very first edition.
	PreviousLetterRef *PreviousLetterRef
}

// MorningLetterSection is a single section in the letter.
type MorningLetterSection struct {
	Key     string
	Title   string
	Bullets []string
	Genre   string
	// Narrative is an optional short paragraph written by the LLM.
	// Empty string when generation failed; bullets remain usable.
	Narrative string
	// WhyReasons parallels Bullets by index. When len != len(Bullets)
	// treat the section as having no why-attribution.
	WhyReasons []WhyReason
}

// PreviousLetterRef captures the minimal shape needed to render the
// Since-yesterday band in the UI.
type PreviousLetterRef struct {
	ID          string
	TargetDate  string
	ThroughLine string
}

// Morning Letter specific why-reason codes that extend the Knowledge Home
// set defined in knowledge_home_item.go. The shared domain.WhyReason type
// is reused so the same WhySurfacedBadge component renders everywhere.
const (
	WhyMorningQuietDay           = "quiet_day"
	WhyMorningSubscriptionUpdate = "subscription_update"
	WhyMorningSupersedeTrail     = "supersede_trail"
)

// MorningLetterSourceEntry is a provenance record linking a section to an article.
type MorningLetterSourceEntry struct {
	LetterID   string
	SectionKey string
	ArticleID  uuid.UUID
	SourceType string
	Position   int
	FeedID     uuid.UUID // Populated by gateway via articles table JOIN
}
