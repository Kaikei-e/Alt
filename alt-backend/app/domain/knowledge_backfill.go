package domain

import (
	"time"

	"github.com/google/uuid"
)

// Backfill job status constants.
const (
	BackfillStatusPending   = "pending"
	BackfillStatusRunning   = "running"
	BackfillStatusPaused    = "paused"
	BackfillStatusCompleted = "completed"
	BackfillStatusFailed    = "failed"
)

// KnowledgeBackfillJob represents a backfill job that replays historical data
// into the knowledge event store for projection.
type KnowledgeBackfillJob struct {
	JobID             uuid.UUID  `json:"job_id" db:"job_id"`
	Status            string     `json:"status" db:"status"`
	ProjectionVersion int        `json:"projection_version" db:"projection_version"`
	CursorUserID      *uuid.UUID `json:"cursor_user_id" db:"cursor_user_id"`
	CursorDate        *time.Time `json:"cursor_date" db:"cursor_date"`
	CursorArticleID   *uuid.UUID `json:"cursor_article_id" db:"cursor_article_id"`
	TotalEvents       int        `json:"total_events" db:"total_events"`
	ProcessedEvents   int        `json:"processed_events" db:"processed_events"`
	ErrorMessage      string     `json:"error_message" db:"error_message"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	StartedAt         *time.Time `json:"started_at" db:"started_at"`
	CompletedAt       *time.Time `json:"completed_at" db:"completed_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}
