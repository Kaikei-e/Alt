package domain

import "time"

// KnowledgeProjectionVersion represents a projection schema version.
type KnowledgeProjectionVersion struct {
	Version     int        `json:"version" db:"version"`
	Description string     `json:"description" db:"description"`
	Status      string     `json:"status" db:"status"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ActivatedAt *time.Time `json:"activated_at" db:"activated_at"`
}
