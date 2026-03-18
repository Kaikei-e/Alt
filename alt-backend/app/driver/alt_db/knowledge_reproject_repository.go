package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateReprojectRun inserts a new reproject run.
// JSONB columns are written as string(jsonBytes) for PgBouncer compatibility (ADR-417).
func (r *AltDBRepository) CreateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateReprojectRun")
	defer span.End()

	query := `INSERT INTO knowledge_reproject_runs
		(reproject_run_id, projection_name, from_version, to_version, initiated_by,
		 mode, status, range_start, range_end, checkpoint_payload,
		 stats_json, diff_summary_json, created_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID, run.ProjectionName, run.FromVersion, run.ToVersion, run.InitiatedBy,
		run.Mode, run.Status, run.RangeStart, run.RangeEnd,
		string(run.CheckpointPayload), string(run.StatsJSON), string(run.DiffSummaryJSON),
		run.CreatedAt, run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("CreateReprojectRun: %w", err)
	}
	return nil
}

// GetReprojectRun retrieves a reproject run by ID.
func (r *AltDBRepository) GetReprojectRun(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.GetReprojectRun")
	defer span.End()

	query := `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE reproject_run_id = $1`

	var run domain.ReprojectRun
	err := r.pool.QueryRow(ctx, query, runID).Scan(
		&run.ReprojectRunID, &run.ProjectionName, &run.FromVersion, &run.ToVersion, &run.InitiatedBy,
		&run.Mode, &run.Status, &run.RangeStart, &run.RangeEnd, &run.CheckpointPayload,
		&run.StatsJSON, &run.DiffSummaryJSON, &run.CreatedAt, &run.StartedAt, &run.FinishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("GetReprojectRun: %w", err)
	}
	return &run, nil
}

// UpdateReprojectRun updates status, stats, diff summary, and timestamps of a reproject run.
// JSONB columns are written as string(jsonBytes) for PgBouncer compatibility (ADR-417).
func (r *AltDBRepository) UpdateReprojectRun(ctx context.Context, run *domain.ReprojectRun) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.UpdateReprojectRun")
	defer span.End()

	query := `UPDATE knowledge_reproject_runs SET
		status = $2, stats_json = $3, diff_summary_json = $4,
		started_at = $5, finished_at = $6
		WHERE reproject_run_id = $1`

	_, err := r.pool.Exec(ctx, query,
		run.ReprojectRunID,
		run.Status,
		string(run.StatsJSON), string(run.DiffSummaryJSON),
		run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("UpdateReprojectRun: %w", err)
	}
	return nil
}

// ListReprojectRuns returns reproject runs ordered by created_at DESC.
// When statusFilter is non-empty, only runs with that status are returned.
func (r *AltDBRepository) ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListReprojectRuns")
	defer span.End()

	var (
		query string
		args  []interface{}
	)

	if statusFilter != "" {
		query = `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE status = $1
		ORDER BY created_at DESC LIMIT $2`
		args = []interface{}{statusFilter, limit}
	} else {
		query = `SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs
		ORDER BY created_at DESC LIMIT $1`
		args = []interface{}{limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListReprojectRuns: %w", err)
	}
	defer rows.Close()

	var runs []domain.ReprojectRun
	for rows.Next() {
		var run domain.ReprojectRun
		err := rows.Scan(
			&run.ReprojectRunID, &run.ProjectionName, &run.FromVersion, &run.ToVersion, &run.InitiatedBy,
			&run.Mode, &run.Status, &run.RangeStart, &run.RangeEnd, &run.CheckpointPayload,
			&run.StatsJSON, &run.DiffSummaryJSON, &run.CreatedAt, &run.StartedAt, &run.FinishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ListReprojectRuns scan: %w", err)
		}
		runs = append(runs, run)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(runs)))
	return runs, nil
}

// CompareProjections compares two projection versions by running aggregate queries
// on knowledge_home_items for each version.
func (r *AltDBRepository) CompareProjections(ctx context.Context, fromVersion, toVersion string) (*domain.ReprojectDiffSummary, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CompareProjections")
	defer span.End()

	summary := &domain.ReprojectDiffSummary{}

	// Fetch stats for "from" version
	fromCount, fromAvg, fromEmpty, err := r.queryVersionStats(ctx, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections from stats: %w", err)
	}
	summary.FromItemCount = fromCount
	summary.FromAvgScore = fromAvg
	summary.FromEmptyCount = fromEmpty

	// Fetch why distribution for "from" version
	summary.FromWhyDistribution, err = r.queryWhyDistribution(ctx, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections from why: %w", err)
	}

	// Fetch stats for "to" version
	toCount, toAvg, toEmpty, err := r.queryVersionStats(ctx, toVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections to stats: %w", err)
	}
	summary.ToItemCount = toCount
	summary.ToAvgScore = toAvg
	summary.ToEmptyCount = toEmpty

	// Fetch why distribution for "to" version
	summary.ToWhyDistribution, err = r.queryWhyDistribution(ctx, toVersion)
	if err != nil {
		return nil, fmt.Errorf("CompareProjections to why: %w", err)
	}

	return summary, nil
}

// queryVersionStats fetches aggregate stats for a given projection version.
func (r *AltDBRepository) queryVersionStats(ctx context.Context, version string) (int64, float64, int64, error) {
	query := `SELECT COUNT(*) AS item_count,
		COALESCE(AVG(score), 0) AS avg_score,
		COUNT(*) FILTER (WHERE summary_excerpt = '' OR summary_excerpt IS NULL) AS empty_count
		FROM knowledge_home_items WHERE projection_version = $1`

	var itemCount, emptyCount int64
	var avgScore float64
	err := r.pool.QueryRow(ctx, query, version).Scan(&itemCount, &avgScore, &emptyCount)
	if err != nil {
		return 0, 0, 0, err
	}
	return itemCount, avgScore, emptyCount, nil
}

// queryWhyDistribution fetches the why-reason code distribution for a given projection version.
func (r *AltDBRepository) queryWhyDistribution(ctx context.Context, version string) (map[string]int64, error) {
	query := `SELECT r.value->>'code' AS reason_code, COUNT(*) AS count
		FROM knowledge_home_items i, jsonb_array_elements(i.why_json) AS r(value)
		WHERE i.projection_version = $1
		GROUP BY reason_code`

	rows, err := r.pool.Query(ctx, query, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dist := make(map[string]int64)
	for rows.Next() {
		var code string
		var count int64
		if err := rows.Scan(&code, &count); err != nil {
			return nil, err
		}
		dist[code] = count
	}
	return dist, nil
}

// CreateProjectionAudit inserts a new projection audit result.
// JSONB columns are written as string(jsonBytes) for PgBouncer compatibility (ADR-417).
func (r *AltDBRepository) CreateProjectionAudit(ctx context.Context, audit *domain.ProjectionAudit) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.CreateProjectionAudit")
	defer span.End()

	query := `INSERT INTO knowledge_projection_audits
		(audit_id, projection_name, projection_version, checked_at,
		 sample_size, mismatch_count, details_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	detailsStr := string(audit.DetailsJSON)
	if detailsStr == "" {
		detailsStr = "{}"
	}

	_, err := r.pool.Exec(ctx, query,
		audit.AuditID, audit.ProjectionName, audit.ProjectionVersion, audit.CheckedAt,
		audit.SampleSize, audit.MismatchCount, detailsStr,
	)
	if err != nil {
		return fmt.Errorf("CreateProjectionAudit: %w", err)
	}
	return nil
}

// ListProjectionAudits returns projection audits for a given projection name,
// ordered by checked_at DESC.
func (r *AltDBRepository) ListProjectionAudits(ctx context.Context, projectionName string, limit int) ([]domain.ProjectionAudit, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListProjectionAudits")
	defer span.End()

	query := `SELECT audit_id, projection_name, projection_version, checked_at,
		sample_size, mismatch_count, details_json
		FROM knowledge_projection_audits
		WHERE projection_name = $1
		ORDER BY checked_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, query, projectionName, limit)
	if err != nil {
		return nil, fmt.Errorf("ListProjectionAudits: %w", err)
	}
	defer rows.Close()

	var audits []domain.ProjectionAudit
	for rows.Next() {
		var audit domain.ProjectionAudit
		err := rows.Scan(
			&audit.AuditID, &audit.ProjectionName, &audit.ProjectionVersion, &audit.CheckedAt,
			&audit.SampleSize, &audit.MismatchCount, &audit.DetailsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("ListProjectionAudits scan: %w", err)
		}
		audits = append(audits, audit)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(audits)))
	return audits, nil
}

// Compile-time interface satisfaction checks.
var _ json.Marshaler = (*json.RawMessage)(nil) // ensure json.RawMessage is available
