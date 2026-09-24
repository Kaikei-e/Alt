package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BackfillJob represents a backfill job record.
type BackfillJob struct {
	JobID             uuid.UUID
	Status            string
	Kind              string
	ProjectionVersion int
	CursorUserID      *uuid.UUID
	CursorDate        *time.Time
	CursorArticleID   *uuid.UUID
	TotalEvents       int
	ProcessedEvents   int
	ErrorMessage      string
	CreatedAt         time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	UpdatedAt         time.Time
}

// RecallSignal represents a user interaction signal for recall scoring.
type RecallSignal struct {
	SignalID       uuid.UUID
	UserID         uuid.UUID
	ItemKey        string
	SignalType     string
	SignalStrength float64
	OccurredAt     time.Time
	Payload        json.RawMessage
}

// GetBackfillJob returns a backfill job by ID.
func (r *Repository) GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*BackfillJob, error) {
	query := `SELECT job_id, status, kind, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at
		FROM knowledge_backfill_jobs WHERE job_id = $1`

	j, err := scanBackfillJob(r.pool.QueryRow(ctx, query, jobID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBackfillJob: %w", err)
	}
	return &j, nil
}

// ListBackfillJobs returns all backfill jobs.
func (r *Repository) ListBackfillJobs(ctx context.Context) ([]BackfillJob, error) {
	query := `SELECT job_id, status, kind, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at
		FROM knowledge_backfill_jobs ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListBackfillJobs: %w", err)
	}
	defer rows.Close()

	return scanBackfillJobs(rows)
}

// CreateBackfillJob inserts a new backfill job.
func (r *Repository) CreateBackfillJob(ctx context.Context, j BackfillJob) error {
	query := `INSERT INTO knowledge_backfill_jobs
		(job_id, status, kind, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		 total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := r.pool.Exec(ctx, query, buildCreateBackfillJobArgs(j)...)
	if err != nil {
		return fmt.Errorf("CreateBackfillJob: %w", err)
	}
	return nil
}

// UpdateBackfillJob updates a backfill job.
func (r *Repository) UpdateBackfillJob(ctx context.Context, j BackfillJob) error {
	query := `UPDATE knowledge_backfill_jobs SET
		status = $2, cursor_user_id = $3, cursor_date = $4, cursor_article_id = $5,
		total_events = $6, processed_events = $7, error_message = $8,
		started_at = $9, completed_at = $10, updated_at = now()
		WHERE job_id = $1`
	_, err := r.pool.Exec(ctx, query, buildUpdateBackfillJobArgs(j)...)
	if err != nil {
		return fmt.Errorf("UpdateBackfillJob: %w", err)
	}
	return nil
}

// ListRecallSignalsByUser returns recall signals for a user since N days ago.
func (r *Repository) ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]RecallSignal, error) {
	since := time.Now().AddDate(0, 0, -sinceDays)
	query := `SELECT signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload
		FROM recall_signals WHERE user_id = $1 AND occurred_at >= $2
		ORDER BY occurred_at DESC`

	rows, err := r.pool.Query(ctx, query, userID, since)
	if err != nil {
		return nil, fmt.Errorf("ListRecallSignalsByUser: %w", err)
	}
	defer rows.Close()

	return scanRecallSignals(rows)
}

// AppendRecallSignal inserts a new recall signal.
func (r *Repository) AppendRecallSignal(ctx context.Context, s RecallSignal) error {
	query := `INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query, buildAppendRecallSignalArgs(s)...)
	if err != nil {
		return fmt.Errorf("AppendRecallSignal: %w", err)
	}
	return nil
}

// kind defaults to 'articles' so legacy producers (proto v1 clients with
// no kind field set) keep their original semantics.
func defaultBackfillJobKind(kind string) string {
	if kind == "" {
		return "articles"
	}
	return kind
}

// Column DEFAULT '{}' is not applied when NULL is sent explicitly:
// recall_signals.payload is JSONB NOT NULL DEFAULT '{}'. Because the INSERT
// names the column unconditionally, a request that omits it binds an explicit NULL,
// defeating the DEFAULT and violating the NOT NULL constraint.
func defaultRecallSignalPayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage([]byte("{}"))
	}
	return payload
}

func scanBackfillJob(row rowScanner) (BackfillJob, error) {
	var j BackfillJob
	err := row.Scan(
		&j.JobID, &j.Status, &j.Kind, &j.ProjectionVersion, &j.CursorUserID, &j.CursorDate, &j.CursorArticleID,
		&j.TotalEvents, &j.ProcessedEvents, &j.ErrorMessage,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt, &j.UpdatedAt,
	)
	return j, err
}

func scanBackfillJobs(rows pgx.Rows) ([]BackfillJob, error) {
	var jobs []BackfillJob
	for rows.Next() {
		j, err := scanBackfillJob(rows)
		if err != nil {
			return nil, fmt.Errorf("ListBackfillJobs scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBackfillJobs rows: %w", err)
	}
	return jobs, nil
}

func scanRecallSignal(row rowScanner) (RecallSignal, error) {
	var s RecallSignal
	err := row.Scan(&s.SignalID, &s.UserID, &s.ItemKey, &s.SignalType, &s.SignalStrength, &s.OccurredAt, &s.Payload)
	return s, err
}

func scanRecallSignals(rows pgx.Rows) ([]RecallSignal, error) {
	var signals []RecallSignal
	for rows.Next() {
		s, err := scanRecallSignal(rows)
		if err != nil {
			return nil, fmt.Errorf("ListRecallSignalsByUser scan: %w", err)
		}
		signals = append(signals, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListRecallSignalsByUser rows: %w", err)
	}
	return signals, nil
}

func buildCreateBackfillJobArgs(j BackfillJob) []any {
	return []any{
		j.JobID, j.Status, defaultBackfillJobKind(j.Kind), j.ProjectionVersion, j.CursorUserID, j.CursorDate, j.CursorArticleID,
		j.TotalEvents, j.ProcessedEvents, j.ErrorMessage,
		j.CreatedAt, j.StartedAt, j.CompletedAt, j.UpdatedAt,
	}
}

func buildUpdateBackfillJobArgs(j BackfillJob) []any {
	return []any{
		j.JobID, j.Status, j.CursorUserID, j.CursorDate, j.CursorArticleID,
		j.TotalEvents, j.ProcessedEvents, j.ErrorMessage,
		j.StartedAt, j.CompletedAt,
	}
}

func buildAppendRecallSignalArgs(s RecallSignal) []any {
	return []any{
		s.SignalID, s.UserID, s.ItemKey, s.SignalType, s.SignalStrength, s.OccurredAt, defaultRecallSignalPayload(s.Payload),
	}
}
