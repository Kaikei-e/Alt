package models

import (
	"time"

	"github.com/google/uuid"
)

// SummarizeJobStatus represents the status of a summarization job
type SummarizeJobStatus string

const (
	SummarizeJobStatusPending    SummarizeJobStatus = "pending"
	SummarizeJobStatusRunning    SummarizeJobStatus = "running"
	SummarizeJobStatusCompleted  SummarizeJobStatus = "completed"
	SummarizeJobStatusFailed     SummarizeJobStatus = "failed"
	SummarizeJobStatusDeadLetter SummarizeJobStatus = "dead_letter"
)

// SummarizeJob represents a job in the summarization queue
type SummarizeJob struct {
	ID           int                `db:"id"`
	JobID        uuid.UUID          `db:"job_id"`
	ArticleID    string             `db:"article_id"`
	Status       SummarizeJobStatus `db:"status"`
	Summary      *string            `db:"summary"`       // Nullable
	ErrorMessage *string            `db:"error_message"` // Nullable
	RetryCount   int                `db:"retry_count"`
	MaxRetries   int                `db:"max_retries"`
	CreatedAt    time.Time          `db:"created_at"`
	StartedAt    *time.Time         `db:"started_at"`
	CompletedAt  *time.Time         `db:"completed_at"`
}

// IsTerminal returns true if the job status is terminal (completed, failed, or dead_letter)
func (j *SummarizeJob) IsTerminal() bool {
	return j.Status == SummarizeJobStatusCompleted || j.Status == SummarizeJobStatusFailed || j.Status == SummarizeJobStatusDeadLetter
}

// CanRetry returns true if the job can be retried
func (j *SummarizeJob) CanRetry() bool {
	return j.Status == SummarizeJobStatusFailed && j.RetryCount < j.MaxRetries
}
