package domain

// RecapDocument represents a recap genre document for Meilisearch indexing.
type RecapDocument struct {
	ID         string   `json:"id"`          // Composite: job_id__genre
	JobID      string   `json:"job_id"`      // Recap job UUID
	ExecutedAt string   `json:"executed_at"` // RFC3339 timestamp
	WindowDays int      `json:"window_days"` // 3 or 7
	Genre      string   `json:"genre"`       // Genre name
	Summary    string   `json:"summary"`     // Summary text
	TopTerms   []string `json:"top_terms"`   // Keywords from c-TF-IDF cluster extraction
	Tags       []string `json:"tags"`        // Semantic tags from tag-generator (KeyBERT)
	Bullets    []string `json:"bullets"`     // Bullet points
}
