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
}

// MorningLetterSection is a single section in the letter.
type MorningLetterSection struct {
	Key     string
	Title   string
	Bullets []string
	Genre   string
}

// MorningLetterSourceEntry is a provenance record linking a section to an article.
type MorningLetterSourceEntry struct {
	LetterID   string
	SectionKey string
	ArticleID  uuid.UUID
	SourceType string
	Position   int
	FeedID     uuid.UUID // Populated by gateway via articles table JOIN
}
