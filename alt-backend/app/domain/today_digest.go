package domain

import (
	"time"

	"github.com/google/uuid"
)

// Freshness constants for TodayDigest.
const (
	FreshnessFresh   = "fresh"
	FreshnessStale   = "stale"
	FreshnessUnknown = "unknown"
)

// FreshnessThreshold is the duration after which a digest is considered stale.
const FreshnessThreshold = 5 * time.Minute

// TodayDigest contains daily summary statistics for a user.
type TodayDigest struct {
	UserID                uuid.UUID  `json:"user_id" db:"user_id"`
	DigestDate            time.Time  `json:"digest_date" db:"digest_date"`
	NewArticles           int        `json:"new_articles" db:"new_articles"`
	SummarizedArticles    int        `json:"summarized_articles" db:"summarized_articles"`
	UnsummarizedArticles  int        `json:"unsummarized_articles" db:"unsummarized_articles"`
	TopTags               []string   `json:"top_tags"`
	PulseRefs             []string   `json:"pulse_refs"`
	WeeklyRecapAvailable  bool       `json:"weekly_recap_available"`
	EveningPulseAvailable bool       `json:"evening_pulse_available"`
	NeedToKnowCount       int        `json:"need_to_know_count"`
	DigestFreshness       string     `json:"digest_freshness"`
	LastProjectedAt       *time.Time `json:"last_projected_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// ComputeFreshness returns "fresh", "stale", or "unknown" based on LastProjectedAt.
func (d TodayDigest) ComputeFreshness(now time.Time) string {
	if d.LastProjectedAt == nil || d.LastProjectedAt.IsZero() {
		return FreshnessUnknown
	}
	if now.Sub(*d.LastProjectedAt) > FreshnessThreshold {
		return FreshnessStale
	}
	return FreshnessFresh
}
