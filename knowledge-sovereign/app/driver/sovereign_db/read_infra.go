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
		if errors.Is(err, pgx.ErrNoRows) {
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProjectionVersions rows: %w", err)
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

// ActivateProjectionVersion sets a version as active and deactivates all
// others, atomically. A mid-failure (or an invalid version argument) must
// never leave zero active versions — that would silently regress every
// reader's COALESCE(...,1) fallback to projection v1. So this: (1) checks
// the target version exists BEFORE touching anything, and (2) performs the
// deactivate+activate pair inside a single transaction so a crash between
// the two statements can never be observed as "no active version".
func (r *Repository) ActivateProjectionVersion(ctx context.Context, version int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	existsQuery := `SELECT 1 FROM knowledge_projection_versions WHERE version = $1`
	var exists int
	if err := tx.QueryRow(ctx, existsQuery, version).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ActivateProjectionVersion: version %d not found", version)
		}
		return fmt.Errorf("ActivateProjectionVersion exists check: %w", err)
	}

	deactivateQuery := `UPDATE knowledge_projection_versions SET status = 'inactive', activated_at = NULL
		WHERE status = 'active' AND version != $1`
	if _, err := tx.Exec(ctx, deactivateQuery, version); err != nil {
		return fmt.Errorf("ActivateProjectionVersion deactivate: %w", err)
	}

	activateQuery := `UPDATE knowledge_projection_versions SET status = 'active', activated_at = now() WHERE version = $1`
	commandTag, err := tx.Exec(ctx, activateQuery, version)
	if err != nil {
		return fmt.Errorf("ActivateProjectionVersion: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("ActivateProjectionVersion: version %d not found", version)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ActivateProjectionVersion commit: %w", err)
	}
	return nil
}

// === Projection checkpoints ===

// ProjectionCheckpoint is one projector's cursor read as an
// optimistic-concurrency token: the sequence it stands at, plus enough of the
// row's identity to tell whether anybody has written it since.
//
// UpdatedAt is that witness, and it is load-bearing rather than informational.
// last_event_seq on its own cannot distinguish "still the 0 I read" from "reset
// to 0 again by a second rebuild while I was folding" — and re-running a
// rebuild is exactly what the reproject runbooks tell an operator to do when
// they are unsure the first one landed. Exists is the second witness: a
// projector that has never run has no row at all, and a rebuild inserts the row
// it did not find, so "absent" and "present holding 0" are different states.
//
// Obtain one from ReadProjectionCheckpointForAdvance; a hand-built value
// describes a row nobody read.
type ProjectionCheckpoint struct {
	LastEventSeq int64
	UpdatedAt    time.Time
	Exists       bool
}

// GetProjectionCheckpoint returns the last processed event sequence for a
// projector, reporting one that has never run as 0. This is the WIRE-level
// form: the GetProjectionCheckpoint RPC forwards the value verbatim.
//
// In-process projectors MUST read with ReadProjectionCheckpointForAdvance
// instead. A bare int64 collapses "no row yet" into 0 and carries nothing that
// says when the row was last written, so it cannot be paired with a
// compare-and-set — see AdvanceProjectionCheckpointIfUnchanged for why a
// projector needs one.
func (r *Repository) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	query := `SELECT last_event_seq FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var seq int64
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&seq)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("GetProjectionCheckpoint: %w", err)
	}
	return seq, nil
}

// UpdateProjectionCheckpoint upserts the projection checkpoint unconditionally.
// This is the WIRE-level form: the UpdateProjectionCheckpoint RPC forwards the
// sequence verbatim, and alt-backend's reproject swap calls it to *set* a
// checkpoint it chose, which is only expressible as an unconditional write.
//
// In-process projectors MUST use ReadProjectionCheckpointForAdvance +
// AdvanceProjectionCheckpointIfUnchanged instead. Unconditional here means a
// batch that read the checkpoint before a concurrent RebuildProjection will
// cheerfully restore the pre-rebuild sequence on top of the read models that
// rebuild just emptied, and the projector then only ever fetches events beyond
// it: PM-2026-010, ~326 articles left unprojected behind a checkpoint standing
// at a tip the projection had never reached. RebuildProjection's
// SELECT ... FOR UPDATE makes such a write wait. It does not make it re-read.
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

// ReadProjectionCheckpointForAdvance reads a projector's checkpoint as the
// token AdvanceProjectionCheckpointIfUnchanged compares against. A projector
// that has never run yields the zero token (Exists false), which is a state in
// its own right and not a stored 0.
func (r *Repository) ReadProjectionCheckpointForAdvance(ctx context.Context, projectorName string) (ProjectionCheckpoint, error) {
	query := `SELECT last_event_seq, updated_at FROM knowledge_projection_checkpoints WHERE projector_name = $1`
	var cp ProjectionCheckpoint
	err := r.pool.QueryRow(ctx, query, projectorName).Scan(&cp.LastEventSeq, &cp.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectionCheckpoint{}, nil
		}
		return ProjectionCheckpoint{}, fmt.Errorf("ReadProjectionCheckpointForAdvance %q: %w", projectorName, err)
	}
	cp.Exists = true
	return cp, nil
}

// AdvanceProjectionCheckpointIfUnchanged moves a projector's checkpoint to
// toSeq only if the stored row is still exactly the state `from` was read from,
// and reports whether it applied. It is the in-process counterpart of the
// wire-facing UpdateProjectionCheckpoint, in the same relation
// AppendKnowledgeEventIfNew stands in to AppendKnowledgeEvent.
//
// A projector reads its checkpoint, spends a batch folding events, and only
// then writes the new sequence back. Anything that writes the row inside that
// window — an operator's RebuildProjection, another service's reproject swap —
// has decided where the projector should stand, and a batch working from the
// older view must not overwrite that decision. Guarding on the sequence *and*
// the updated_at witness makes both a reset to a different sequence and a reset
// to the same sequence visible; guarding on the row's absence makes the
// first-ever batch safe against a rebuild that created the row underneath it,
// which is the case RebuildProjection's row lock cannot cover because there is
// no row to lock.
//
// A rejected advance is a normal outcome, not a failure: it is reported as
// (false, nil) rather than an error, so callers must branch on applied rather
// than on err — the same shape, and the same reason, as
// AppendKnowledgeEventIfNew. Callers must not retry; the stored checkpoint is
// authoritative and the next batch re-reads it.
//
// Two writes would have to land in the same microsecond *and* choose the same
// sequence for the witness to mistake them for no write at all.
func (r *Repository) AdvanceProjectionCheckpointIfUnchanged(
	ctx context.Context,
	projectorName string,
	from ProjectionCheckpoint,
	toSeq int64,
) (bool, error) {
	if !from.Exists {
		// Insert-only. ON CONFLICT DO UPDATE here would overwrite precisely
		// the row a concurrent rebuild had just created.
		insertQuery := `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
			VALUES ($1, $2, now())
			ON CONFLICT (projector_name) DO NOTHING`
		tag, err := r.pool.Exec(ctx, insertQuery, projectorName, toSeq)
		if err != nil {
			return false, fmt.Errorf("AdvanceProjectionCheckpointIfUnchanged insert %q: %w", projectorName, err)
		}
		return tag.RowsAffected() == 1, nil
	}

	casQuery := `UPDATE knowledge_projection_checkpoints
		SET last_event_seq = $3, updated_at = now()
		WHERE projector_name = $1 AND last_event_seq = $2 AND updated_at = $4`
	tag, err := r.pool.Exec(ctx, casQuery, projectorName, from.LastEventSeq, toSeq, from.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("AdvanceProjectionCheckpointIfUnchanged %q: %w", projectorName, err)
	}
	return tag.RowsAffected() == 1, nil
}

// GetProjectionLag returns how many events the farthest-behind projector
// checkpoint trails the knowledge_events tip (max(event_seq) - min(checkpoint)).
func (r *Repository) GetProjectionLag(ctx context.Context) (float64, error) {
	query := `SELECT GREATEST(
		(SELECT COALESCE(MAX(event_seq), 0) FROM knowledge_events) -
		(SELECT COALESCE(MIN(last_event_seq), 0) FROM knowledge_projection_checkpoints),
		0
	)::float8`
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
		if errors.Is(err, pgx.ErrNoRows) {
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListReprojectRuns rows: %w", err)
	}
	return runs, nil
}

// CreateReprojectRun inserts a new reproject run.
//
// The JSONB columns (checkpoint_payload / stats_json / diff_summary_json) are
// NOT NULL with DEFAULT '{}'. PostgreSQL only applies DEFAULT when a column
// is omitted from the INSERT — passing a nil json.RawMessage explicitly
// sends NULL, which trips the NOT NULL constraint. Normalise nil → '{}' here
// so any caller (RPC client, DB-direct, future migrations) can safely leave
// these unset.
func (r *Repository) CreateReprojectRun(ctx context.Context, run ReprojectRun) error {
	query := `INSERT INTO knowledge_reproject_runs
		(reproject_run_id, projection_name, from_version, to_version, initiated_by,
		 mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
		 created_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID, run.ProjectionName, run.FromVersion, run.ToVersion, run.InitiatedBy,
		run.Mode, run.Status, run.RangeStart, run.RangeEnd,
		emptyJSONIfNil(run.CheckpointPayload), emptyJSONIfNil(run.StatsJSON), emptyJSONIfNil(run.DiffSummaryJSON),
		run.CreatedAt, run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("CreateReprojectRun: %w", err)
	}
	return nil
}

// emptyJSONIfNil returns the canonical empty JSON object when the input is
// nil or empty. NOT NULL JSONB columns with DEFAULT '{}' need an explicit
// value when listed in the INSERT column list — DEFAULT does not fire for
// values that are sent as NULL.
func emptyJSONIfNil(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage([]byte(`{}`))
	}
	return v
}

// UpdateReprojectRun updates a reproject run.
func (r *Repository) UpdateReprojectRun(ctx context.Context, run ReprojectRun) error {
	query := `UPDATE knowledge_reproject_runs SET
		status = $2, checkpoint_payload = $3, stats_json = $4, diff_summary_json = $5,
		started_at = $6, finished_at = $7
		WHERE reproject_run_id = $1`
	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID, run.Status,
		emptyJSONIfNil(run.CheckpointPayload), emptyJSONIfNil(run.StatsJSON), emptyJSONIfNil(run.DiffSummaryJSON),
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProjectionAudits rows: %w", err)
	}
	return audits, nil
}

// CreateProjectionAudit inserts an audit record. details_json is JSONB
// NOT NULL DEFAULT '{}' — same rationale as CreateReprojectRun: pgx sends
// nil json.RawMessage as NULL, which trips the constraint instead of
// activating the DEFAULT. emptyJSONIfNil normalises nil → '{}' so callers
// (including ones that skip verification when comparePort == nil) cannot
// silent-fail the INSERT.
func (r *Repository) CreateProjectionAudit(ctx context.Context, audit ProjectionAudit) error {
	query := `INSERT INTO knowledge_projection_audits
		(audit_id, projection_name, projection_version, checked_at, sample_size, mismatch_count, details_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, query,
		audit.AuditID, audit.ProjectionName, audit.ProjectionVersion, audit.CheckedAt,
		audit.SampleSize, audit.MismatchCount, emptyJSONIfNil(audit.DetailsJSON),
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
		&j.JobID, &j.Status, &j.Kind, &j.ProjectionVersion, &j.CursorUserID, &j.CursorDate, &j.CursorArticleID,
		&j.TotalEvents, &j.ProcessedEvents, &j.ErrorMessage,
		&j.CreatedAt, &j.StartedAt, &j.CompletedAt, &j.UpdatedAt,
	)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListBackfillJobs rows: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListRecallSignalsByUser rows: %w", err)
	}
	return signals, nil
}

// AppendRecallSignal inserts a new recall signal.
func (r *Repository) AppendRecallSignal(ctx context.Context, s RecallSignal) error {
	query := `INSERT INTO recall_signals (signal_id, user_id, item_key, signal_type, signal_strength, occurred_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	// `payload` is JSONB NOT NULL DEFAULT '{}' — the schema saying the field is
	// optional. Because the INSERT names the column unconditionally, a request
	// that omits it (nil on the wire) would bind an explicit NULL, the DEFAULT
	// would never apply, and the insert would die on the NOT NULL constraint —
	// surfacing to the caller as Connect `internal`, i.e. a malformed-looking
	// request reported as a broken service. Default here so the column's own
	// contract holds.
	payload := s.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := r.pool.Exec(ctx, query, s.SignalID, s.UserID, s.ItemKey, s.SignalType, s.SignalStrength, s.OccurredAt, payload)
	if err != nil {
		return fmt.Errorf("AppendRecallSignal: %w", err)
	}
	return nil
}
