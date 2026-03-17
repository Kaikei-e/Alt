package domain

import (
	"time"

	"github.com/google/uuid"
)

// SummaryVersion represents a versioned summary artifact.
type SummaryVersion struct {
	SummaryVersionID uuid.UUID  `json:"summary_version_id" db:"summary_version_id"`
	ArticleID        uuid.UUID  `json:"article_id" db:"article_id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	GeneratedAt      time.Time  `json:"generated_at" db:"generated_at"`
	Model            string     `json:"model" db:"model"`
	PromptVersion    string     `json:"prompt_version" db:"prompt_version"`
	InputHash        string     `json:"input_hash" db:"input_hash"`
	QualityScore     *float64   `json:"quality_score" db:"quality_score"`
	SummaryText      string     `json:"summary_text" db:"summary_text"`
	SupersededBy     *uuid.UUID `json:"superseded_by" db:"superseded_by"`
}
