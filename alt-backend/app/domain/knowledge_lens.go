package domain

import (
	"time"

	"github.com/google/uuid"
)

// KnowledgeLens represents a saved viewpoint for the knowledge stream.
type KnowledgeLens struct {
	LensID      uuid.UUID  `json:"lens_id" db:"lens_id"`
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name        string     `json:"name" db:"name"`
	Description string     `json:"description" db:"description"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at" db:"archived_at"`

	CurrentVersion *KnowledgeLensVersion `json:"current_version,omitempty"`
}

// KnowledgeLensVersion represents a version of a lens configuration.
type KnowledgeLensVersion struct {
	LensVersionID  uuid.UUID  `json:"lens_version_id" db:"lens_version_id"`
	LensID         uuid.UUID  `json:"lens_id" db:"lens_id"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	QueryText      string     `json:"query_text" db:"query_text"`
	TagIDs         []string   `json:"tag_ids"`
	TimeWindow     string     `json:"time_window" db:"time_window"`
	IncludeRecap   bool       `json:"include_recap" db:"include_recap"`
	IncludePulse   bool       `json:"include_pulse" db:"include_pulse"`
	SortMode       string     `json:"sort_mode" db:"sort_mode"`
	SupersededBy   *uuid.UUID `json:"superseded_by" db:"superseded_by"`
}

// KnowledgeCurrentLens tracks which lens is active for a user.
type KnowledgeCurrentLens struct {
	UserID        uuid.UUID `json:"user_id" db:"user_id"`
	LensID        uuid.UUID `json:"lens_id" db:"lens_id"`
	LensVersionID uuid.UUID `json:"lens_version_id" db:"lens_version_id"`
	SelectedAt    time.Time `json:"selected_at" db:"selected_at"`
}
