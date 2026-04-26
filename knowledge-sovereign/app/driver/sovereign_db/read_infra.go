package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProjectionVersion represents a projection version record.
type ProjectionVersion struct {
	Version     int
	Description string
	Status      string
	CreatedAt   time.Time
	ActivatedAt *time.Time
}

// ReprojectRun represents a re-projection run.
type ReprojectRun struct {
	ReprojectRunID    uuid.UUID
	ProjectionName    string
	FromVersion       string
	ToVersion         string
	InitiatedBy       *uuid.UUID
	Mode              string
	Status            string
	RangeStart        *time.Time
	RangeEnd          *time.Time
	CheckpointPayload json.RawMessage
	StatsJSON         json.RawMessage
	DiffSummaryJSON   json.RawMessage
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

// ProjectionAudit represents an audit record.
type ProjectionAudit struct {
	AuditID           uuid.UUID
	ProjectionName    string
	ProjectionVersion string
	CheckedAt         time.Time
	SampleSize        int
	MismatchCount     int
	DetailsJSON       json.RawMessage
}

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

// ReprojectDiffSummary represents comparison stats between projection versions.
type ReprojectDiffSummary struct {
	FromCount        int
	ToCount          int
	FromAvgScore     float64
	ToAvgScore       float64
	FromEmptySummary int
	ToEmptySummary   int
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

// === Projection versions ===

// GetActiveProjectionVersion returns the currently active projection version.
func (r *Repository) GetActiveProjectionVersion(ctx context.Context) (*ProjectionVersion, error) {
	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions WHERE status = 'active'
		ORDER BY version DESC LIMIT 1`

	var v ProjectionVersion
	err := r.pool.QueryRow(ctx, query).Scan(&v.Version, &v.Description, &v.Status, &v.CreatedAt, &v.ActivatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetActiveProjectionVersion: %w", err)
	}
	return &v, nil
}

// ListProjectionVersions returns all projection versions.
func (r *Repository) ListProjectionVersions(ctx context.Context) ([]ProjectionVersion, error) {
	query := `SELECT version, description, status, created_at, activated_at
		FROM knowledge_projection_versions ORDER BY version DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListProjectionVersions: %w", err)
	}
	defer rows.Close()

	var versions []ProjectionVersion
	for rows.Next() {
		var v ProjectionVersion
		if err := rows.Scan(&v.Version, &v.Description, &v.Status, &v.CreatedAt, &v.ActivatedAt); err != nil {
			return nil, fmt.Errorf("ListProjectionVersions scan: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// CreateProjectionVersion inserts a new projection version.
func (r *Repository) CreateProjectionVersion(ctx context.Context, v ProjectionVersion) error {
	query := `INSERT INTO knowledge_projection_versions (version, description, status, created_at, activated_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, query, v.Version, v.Description, v.Status, v.CreatedAt, v.ActivatedAt)
	if err != nil {
		return fmt.Errorf("CreateProjectionVersion: %w", err)
	}
	return nil
}

// ActivateProjectionVersion sets a version as active and deactivates all others.
func (r *Repository) ActivateProjectionVersion(ctx context.Context, version int) error {
	query := `UPDATE knowledge_projection_versions SET status = 'inactive', activated_at = NULL WHERE status = 'active'`
	if _, err := r.pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("ActivateProjectionVersion deactivate: %w", err)
	}
	query = `UPDATE knowledge_projection_versions SET status = 'active', activated_at = now() WHERE version = $1`
	commandTag, err := r.pool.Exec(ctx, query, version)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ActivateProjectionVersion: version %d not found", version)
	}
	return nil
}

// === Projection checkpoints ===

// GetProjectionCheckpoint returns the last processed event sequence for a projector.
func (r *Repository) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	query := `SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var seq int64
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&seq)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("GetProjectionCheckpoint: %w", err)
	}
	return seq, nil
}

// UpdateProjectionCheckpoint upserts the projection checkpoint.
func (r *Repository) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	query := `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (projector_name) DO UPDATE SET last_event_seq = $2, updated_at = now()`
	_, err := r.pool.Exec(ctx, query, projectorName, lastSeq)
	if err != nil {
		return fmt.Errorf("UpdateProjectionCheckpoint: %w", err)
	}
	return nil
}

// GetProjectionLag returns the lag in seconds since the last checkpoint update.
func (r *Repository) GetProjectionLag(ctx context.Context) (float64, error) {
	query := `SELECT EXTRACT(EPOCH FROM (now() - COALESCE(MAX(updated_at), now()))) FROM knowledge_projection_checkpoints`
	var lag float64
	if err := r.pool.QueryRow(ctx, query).Scan(&lag); err != nil {
		return 0, fmt.Errorf("GetProjectionLag: %w", err)
	}
	return lag, nil
}

// GetProjectionAge returns the age in seconds since the last checkpoint update.
func (r *Repository) GetProjectionAge(ctx context.Context) (float64, error) {
	query := `SELECT EXTRACT(EPOCH FROM (now() - COALESCE(MAX(updated_at), now()))) FROM knowledge_projection_checkpoints`
	var age float64
	if err := r.pool.QueryRow(ctx, query).Scan(&age); err != nil {
		return 0, fmt.Errorf("GetProjectionAge: %w", err)
	}
	return age, nil
}

// === Reproject runs ===

// GetReprojectRun returns a reproject run by ID.
func (r *Repository) GetReprojectRun(ctx context.Context, runID uuid.UUID) (*ReprojectRun, error) {
	query := `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
		created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE reproject_run_id = $1`

	var run ReprojectRun
	err := r.pool.QueryRow(ctx, query, runID).Scan(
		&run.ReprojectRunID, &run.ProjectionName, &run.FromVersion, &run.ToVersion, &run.InitiatedBy,
		&run.Mode, &run.Status, &run.RangeStart, &run.RangeEnd,
		&run.CheckpointPayload, &run.StatsJSON, &run.DiffSummaryJSON,
		&run.CreatedAt, &run.StartedAt, &run.FinishedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetReprojectRun: %w", err)
	}
	return &run, nil
}

// ListReprojectRuns returns reproject runs with optional status filter.
func (r *Repository) ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]ReprojectRun, error) {
	var query string
	var args []any
	if statusFilter != "" {
		query = `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
			mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
			created_at, started_at, finished_at
			FROM knowledge_reproject_runs WHERE status = $1
			ORDER BY created_at DESC LIMIT $2`
		args = []any{statusFilter, limit}
	} else {
		query = `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
			mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
			created_at, started_at, finished_at
			FROM knowledge_reproject_runs ORDER BY created_at DESC LIMIT $1`
		args = []any{limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListReprojectRuns: %w", err)
	}
	defer rows.Close()

	var runs []ReprojectRun
	for rows.Next() {
		var run ReprojectRun
		if err := rows.Scan(
			&run.ReprojectRunID, &run.ProjectionName, &run.FromVersion, &run.ToVersion, &run.InitiatedBy,
			&run.Mode, &run.Status, &run.RangeStart, &run.RangeEnd,
			&run.CheckpointPayload, &run.StatsJSON, &run.DiffSummaryJSON,
			&run.CreatedAt, &run.StartedAt, &run.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("ListReprojectRuns scan: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// CreateReprojectRun inserts a new reproject run.
func (r *Repository) CreateReprojectRun(ctx context.Context, run ReprojectRun) error {
	query := `INSERT INTO knowledge_reproject_runs
		(reproject_run_id, projection_name, from_version, to_version, initiated_by,
		 mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
		 created_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID, run.ProjectionName, run.FromVersion, run.ToVersion, run.InitiatedBy,
		run.Mode, run.Status, run.RangeStart, run.RangeEnd,
		run.CheckpointPayload, run.StatsJSON, run.DiffSummaryJSON,
		run.CreatedAt, run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("CreateReprojectRun: %w", err)
	}
	return nil
}

// UpdateReprojectRun updates a reproject run.
func (r *Repository) UpdateReprojectRun(ctx context.Context, run ReprojectRun) error {
	query := `UPDATE knowledge_reproject_runs SET
		status = $2, checkpoint_payload = $3, stats_json = $4, diff_summary_json = $5,
		started_at = $6, finished_at = $7
		WHERE reproject_run_id = $1`
	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID, run.Status, run.CheckpointPayload, run.StatsJSON, run.DiffSummaryJSON,
		run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("UpdateReprojectRun: %w", err)
	}
	return nil
}

// CompareProjections compares two projection versions.
func (r *Repository) CompareProjections(ctx context.Context, fromVersion, toVersion string) (*ReprojectDiffSummary, error) {
	fromStats, err := r.queryVersionStats(ctx, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections from: %w", err)
	}
	toStats, err := r.queryVersionStats(ctx, toVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections to: %w", err)
	}
	return &ReprojectDiffSummary{
		FromCount:        fromStats.count,
		ToCount:          toStats.count,
		FromAvgScore:     fromStats.avgScore,
		ToAvgScore:       toStats.avgScore,
		FromEmptySummary: fromStats.emptySummary,
		ToEmptySummary:   toStats.emptySummary,
	}, nil
}

type versionStats struct {
	count        int
	avgScore     float64
	emptySummary int
}

func (r *Repository) queryVersionStats(ctx context.Context, version string) (versionStats, error) {
	query := `SELECT COUNT(*), COALESCE(AVG(score), 0),
		COUNT(*) FILTER (WHERE summary_state = 'missing' OR summary_state = '')
		FROM knowledge_home_items WHERE projection_version = $1::int`
	var s versionStats
	if err := r.pool.QueryRow(ctx, query, version).Scan(&s.count, &s.avgScore, &s.emptySummary); err != nil {
		return versionStats{}, fmt.Errorf("queryVersionStats: %w", err)
	}
	return s, nil
}

// ListProjectionAudits returns audit records.
func (r *Repository) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]ProjectionAudit, error) {
	query := `SELECT audit_id, projection_name, projection_version, checked_at,
		sample_size, mismatch_count, details_json
		FROM knowledge_projection_audits WHERE projection_name = $1
		ORDER BY checked_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, query, projectionName, limit)
	if err != nil {
		return nil, fmt.Errorf("ListProjectionAudits: %w", err)
	}
	defer rows.Close()

	var audits []ProjectionAudit
	for rows.Next() {
		var a ProjectionAudit
		if err := rows.Scan(&a.AuditID, &a.ProjectionName, &a.ProjectionVersion, &a.CheckedAt,
			&a.SampleSize, &a.MismatchCount, &a.DetailsJSON); err != nil {
			return nil, fmt.Errorf("ListProjectionAudits scan: %w", err)
		}
		audits = append(audits, a)
	}
	return audits, nil
}

// CreateProjectionAudit inserts an audit record.
func (r *Repository) CreateProjectionAudit(ctx context.Context, audit ProjectionAudit) error {
	query := `INSERT INTO knowledge_projection_audits
		(audit_id, projection_name, projection_version, checked_at, sample_size, mismatch_count, details_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query,
		audit.AuditID, audit.ProjectionName, audit.ProjectionVersion, audit.CheckedAt,
		audit.SampleSize, audit.MismatchCount, audit.DetailsJSON,
	)
	if err != nil {
		return fmt.Errorf("CreateProjectionAudit: %w", err)
	}
	return nil
}

// === Backfill ===

// GetBackfillJob returns a backfill job by ID.
func (r *Repository) GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*BackfillJob, error) {
	query := `SELECT job_id, status, kind, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at
		FROM knowledge_backfill_jobs WHERE job_id = $1`

	var j BackfillJob
	err := r.pool.QueryRow(ctx, query, jobID).Scan(
		&j.JobID, &j.Status, &j.ProjectionVersion, &j.CursorUserID, &j.CursorDate, &j.CursorArticleID,
		&j.TotalEvents, &j.ProcessedEvents, &j.ErrorMessage,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt, &j.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
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

	var jobs []BackfillJob
	for rows.Next() {
		var j BackfillJob
		if err := rows.Scan(
			&j.JobID, &j.Status, &j.Kind, &j.ProjectionVersion, &j.CursorUserID, &j.CursorDate, &j.CursorArticleID,
			&j.TotalEvents, &j.ProcessedEvents, &j.ErrorMessage,
			&j.CreatedAt, &j.StartedAt, &j.CompletedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ListBackfillJobs scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// CreateBackfillJob inserts a new backfill job.
func (r *Repository) CreateBackfillJob(ctx context.Context, j BackfillJob) error {
	// kind defaults to 'articles' so legacy producers (proto v1 clients with
	// no kind field set) keep their original semantics.
	if j.Kind == "" {
		j.Kind = "articles"
	}
	query := `INSERT INTO knowledge_backfill_jobs
		(job_id, status, kind, projection_version, cursor_user_id, cursor_date, cursor_article_id,
		 total_events, processed_events, error_message, created_at, started_at, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	_, err := r.pool.Exec(ctx, query,
		j.JobID, j.Status, j.Kind, j.ProjectionVersion, j.CursorUserID, j.CursorDate, j.CursorArticleID,
		j.TotalEvents, j.ProcessedEvents, j.ErrorMessage,
		j.CreatedAt, j.StartedAt, j.CompletedAt, j.UpdatedAt,
	)
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
	_, err := r.pool.Exec(ctx, query,
		j.JobID, j.Status, j.CursorUserID, j.CursorDate, j.CursorArticleID,
		j.TotalEvents, j.ProcessedEvents, j.ErrorMessage,
		j.StartedAt, j.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("UpdateBackfillJob: %w", err)
	}
	return nil
}

// === Recall signals ===

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

	var signals []RecallSignal
	for rows.Next() {
		var s RecallSignal
		if err := rows.Scan(&s.SignalID, &s.UserID, &s.ItemKey, &s.SignalType, &s.SignalStrength, &s.OccurredAt, &s.Payload); err != nil {
			return nil, fmt.Errorf("ListRecallSignalsByUser scan: %w", err)
		}
		signals = append(signals, s)
	}
	return signals, nil
}

// AppendRecallSignal inserts a new recall signal.
func (r *Repository) AppendRecallSignal(ctx context.Context, s RecallSignal) error {
	query := `INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query, s.SignalID, s.UserID, s.ItemKey, s.SignalType, s.SignalStrength, s.OccurredAt, s.Payload)
	if err != nil {
		return fmt.Errorf("AppendRecallSignal: %w", err)
	}
	return nil
}
