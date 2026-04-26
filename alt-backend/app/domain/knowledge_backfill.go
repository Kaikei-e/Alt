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

// Backfill job kind discriminator. The knowledge_backfill_jobs table now
// hosts more than one backfill stream; the kind column tells the job runner
// which source rows to walk and which event type to emit.
const (
	BackfillKindArticles          = "articles"
	BackfillKindSummaryNarratives = "summary_narratives"
)

// KnowledgeBackfillJob represents a backfill job that replays historical data
// into the knowledge event store for projection.
type KnowledgeBackfillJob struct {
	JobID             uuid.UUID  `json:"job_id" db:"job_id"`
	Status            string     `json:"status" db:"status"`
	Kind              string     `json:"kind" db:"kind"`
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

// KnowledgeBackfillArticle represents a historical article to be replayed into
// the knowledge event store for projection backfill.
type KnowledgeBackfillArticle struct {
	ArticleID   uuid.UUID
	UserID      uuid.UUID
	CreatedAt   time.Time
	PublishedAt time.Time
	Title       string
	URL         string
}

// KnowledgeBackfillSummaryTitle represents one (summary_version, article)
// pair the summary-narrative-backfill job will emit a discovered event for.
// Title is sourced from the current articles row at backfill time — the
// article_versions snapshot table does not exist yet (ADR-000846 trade-off).
type KnowledgeBackfillSummaryTitle struct {
	SummaryVersionID uuid.UUID
	ArticleID        uuid.UUID
	UserID           uuid.UUID
	TenantID         uuid.UUID
	Title            string
	GeneratedAt      time.Time
}
