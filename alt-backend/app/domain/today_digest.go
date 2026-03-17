package domain

import (
	"time"

	"github.com/google/uuid"
)

// TodayDigest contains daily summary statistics for a user.
type TodayDigest struct {
	UserID                uuid.UUID `json:"user_id" db:"user_id"`
	DigestDate            time.Time `json:"digest_date" db:"digest_date"`
	NewArticles           int       `json:"new_articles" db:"new_articles"`
	SummarizedArticles    int       `json:"summarized_articles" db:"summarized_articles"`
	UnsummarizedArticles  int       `json:"unsummarized_articles" db:"unsummarized_articles"`
	TopTags               []string  `json:"top_tags"`
	WeeklyRecapAvailable  bool      `json:"weekly_recap_available"`
	EveningPulseAvailable bool      `json:"evening_pulse_available"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}
