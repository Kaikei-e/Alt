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

// ReprojectDiffSummary represents comparison stats between projection versions.
type ReprojectDiffSummary struct {
	FromCount        int
	ToCount          int
	FromAvgScore     float64
	ToAvgScore       float64
	FromEmptySummary int
	ToEmptySummary   int
}

type versionStats struct {
	count        int
	avgScore     float64
	emptySummary int
}

// GetReprojectRun returns a reproject run by ID.
func (r *Repository) GetReprojectRun(ctx context.Context, runID uuid.UUID) (*ReprojectRun, error) {
	query := `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload, stats_json, diff_summary_json,
		created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE reproject_run_id = $1`

	run, err := scanReprojectRun(r.pool.QueryRow(ctx, query, runID))
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

	return scanReprojectRuns(rows)
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
	_, err := r.pool.Exec(ctx, query, buildCreateReprojectRunArgs(run)...)
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
	_, err := r.pool.Exec(ctx, query, buildUpdateReprojectRunArgs(run)...)
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
	summary := calcReprojectDiffSummary(fromStats, toStats)
	return &summary, nil
}

func calcReprojectDiffSummary(from, to versionStats) ReprojectDiffSummary {
	return ReprojectDiffSummary{
		FromCount:        from.count,
		ToCount:          to.count,
		FromAvgScore:     from.avgScore,
		ToAvgScore:       to.avgScore,
		FromEmptySummary: from.emptySummary,
		ToEmptySummary:   to.emptySummary,
	}
}

func (r *Repository) queryVersionStats(ctx context.Context, version string) (versionStats, error) {
	query := `SELECT COUNT(*), COALESCE(AVG(score), 0),
		COUNT(*) FILTER (WHERE summary_state = 'missing' OR summary_state = '')
		FROM knowledge_home_items WHERE projection_version = $1::int`
	s, err := scanVersionStats(r.pool.QueryRow(ctx, query, version))
	if err != nil {
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

	return scanProjectionAudits(rows)
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
	_, err := r.pool.Exec(ctx, query, buildCreateProjectionAuditArgs(audit)...)
	if err != nil {
		return fmt.Errorf("CreateProjectionAudit: %w", err)
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

func scanReprojectRun(row rowScanner) (ReprojectRun, error) {
	var run ReprojectRun
	err := row.Scan(
		&run.ReprojectRunID, &run.ProjectionName, &run.FromVersion, &run.ToVersion, &run.InitiatedBy,
		&run.Mode, &run.Status, &run.RangeStart, &run.RangeEnd,
		&run.CheckpointPayload, &run.StatsJSON, &run.DiffSummaryJSON,
		&run.CreatedAt, &run.StartedAt, &run.FinishedAt,
	)
	return run, err
}

func scanReprojectRuns(rows pgx.Rows) ([]ReprojectRun, error) {
	var runs []ReprojectRun
	for rows.Next() {
		run, err := scanReprojectRun(rows)
		if err != nil {
			return nil, fmt.Errorf("ListReprojectRuns scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListReprojectRuns rows: %w", err)
	}
	return runs, nil
}

func scanProjectionAudit(row rowScanner) (ProjectionAudit, error) {
	var a ProjectionAudit
	err := row.Scan(&a.AuditID, &a.ProjectionName, &a.ProjectionVersion, &a.CheckedAt,
		&a.SampleSize, &a.MismatchCount, &a.DetailsJSON)
	return a, err
}

func scanProjectionAudits(rows pgx.Rows) ([]ProjectionAudit, error) {
	var audits []ProjectionAudit
	for rows.Next() {
		a, err := scanProjectionAudit(rows)
		if err != nil {
			return nil, fmt.Errorf("ListProjectionAudits scan: %w", err)
		}
		audits = append(audits, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListProjectionAudits rows: %w", err)
	}
	return audits, nil
}

func scanVersionStats(row rowScanner) (versionStats, error) {
	var s versionStats
	err := row.Scan(&s.count, &s.avgScore, &s.emptySummary)
	return s, err
}

func buildCreateReprojectRunArgs(run ReprojectRun) []any {
	return []any{
		run.ReprojectRunID, run.ProjectionName, run.FromVersion, run.ToVersion, run.InitiatedBy,
		run.Mode, run.Status, run.RangeStart, run.RangeEnd,
		emptyJSONIfNil(run.CheckpointPayload), emptyJSONIfNil(run.StatsJSON), emptyJSONIfNil(run.DiffSummaryJSON),
		run.CreatedAt, run.StartedAt, run.FinishedAt,
	}
}

func buildUpdateReprojectRunArgs(run ReprojectRun) []any {
	return []any{
		run.ReprojectRunID, run.Status,
		emptyJSONIfNil(run.CheckpointPayload), emptyJSONIfNil(run.StatsJSON), emptyJSONIfNil(run.DiffSummaryJSON),
		run.StartedAt, run.FinishedAt,
	}
}

func buildCreateProjectionAuditArgs(audit ProjectionAudit) []any {
	return []any{
		audit.AuditID, audit.ProjectionName, audit.ProjectionVersion, audit.CheckedAt,
		audit.SampleSize, audit.MismatchCount, emptyJSONIfNil(audit.DetailsJSON),
	}
}
