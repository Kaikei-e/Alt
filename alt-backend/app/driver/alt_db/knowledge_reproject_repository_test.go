package alt_db

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAltDBRepository_CreateReprojectRun_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()
	initiatedBy := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	rangeStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

	run := &domain.ReprojectRun{
		ReprojectRunID:    runID,
		ProjectionName:    "knowledge_home",
		FromVersion:       "v1",
		ToVersion:         "v2",
		InitiatedBy:       &initiatedBy,
		Mode:              domain.ReprojectModeFull,
		Status:            domain.ReprojectStatusPending,
		RangeStart:        &rangeStart,
		RangeEnd:          &rangeEnd,
		CheckpointPayload: json.RawMessage(`{"cursor":"abc"}`),
		StatsJSON:         json.RawMessage(`{}`),
		DiffSummaryJSON:   json.RawMessage(`{}`),
		CreatedAt:         now,
		StartedAt:         nil,
		FinishedAt:        nil,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO knowledge_reproject_runs
		(reproject_run_id, projection_name, from_version, to_version, initiated_by,
		 mode, status, range_start, range_end, checkpoint_payload,
		 stats_json, diff_summary_json, created_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`)).
		WithArgs(
			runID, "knowledge_home", "v1", "v2", &initiatedBy,
			domain.ReprojectModeFull, domain.ReprojectStatusPending,
			&rangeStart, &rangeEnd,
			`{"cursor":"abc"}`, `{}`, `{}`,
			now, (*time.Time)(nil), (*time.Time)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.CreateReprojectRun(context.Background(), run)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_CreateReprojectRun_EmptyJSONFields_DefaultToEmptyObjects(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	run := &domain.ReprojectRun{
		ReprojectRunID: runID,
		ProjectionName: "knowledge_home",
		FromVersion:    "v1",
		ToVersion:      "v2",
		Mode:           domain.ReprojectModeFull,
		Status:         domain.ReprojectStatusPending,
		CreatedAt:      now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO knowledge_reproject_runs
		(reproject_run_id, projection_name, from_version, to_version, initiated_by,
		 mode, status, range_start, range_end, checkpoint_payload,
		 stats_json, diff_summary_json, created_at, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`)).
		WithArgs(
			runID, "knowledge_home", "v1", "v2", (*uuid.UUID)(nil),
			domain.ReprojectModeFull, domain.ReprojectStatusPending,
			(*time.Time)(nil), (*time.Time)(nil),
			`{}`, `{}`, `{}`,
			now, (*time.Time)(nil), (*time.Time)(nil),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.CreateReprojectRun(context.Background(), run)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetReprojectRun_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()
	initiatedBy := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows([]string{
		"reproject_run_id", "projection_name", "from_version", "to_version", "initiated_by",
		"mode", "status", "range_start", "range_end", "checkpoint_payload",
		"stats_json", "diff_summary_json", "created_at", "started_at", "finished_at",
	}).AddRow(
		runID, "knowledge_home", "v1", "v2", &initiatedBy,
		domain.ReprojectModeFull, domain.ReprojectStatusRunning,
		nil, nil, json.RawMessage(`{}`),
		json.RawMessage(`{"events_processed":100}`), json.RawMessage(`{}`),
		now, &now, nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE reproject_run_id = $1`)).
		WithArgs(runID).
		WillReturnRows(rows)

	result, err := repo.GetReprojectRun(context.Background(), runID)
	require.NoError(t, err)
	assert.Equal(t, runID, result.ReprojectRunID)
	assert.Equal(t, "knowledge_home", result.ProjectionName)
	assert.Equal(t, "v1", result.FromVersion)
	assert.Equal(t, "v2", result.ToVersion)
	assert.Equal(t, domain.ReprojectStatusRunning, result.Status)
	assert.Equal(t, domain.ReprojectModeFull, result.Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_GetReprojectRun_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}
	runID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE reproject_run_id = $1`)).
		WithArgs(runID).
		WillReturnError(pgx.ErrNoRows)

	result, err := repo.GetReprojectRun(context.Background(), runID)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "GetReprojectRun")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpdateReprojectRun_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()
	now := time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	run := &domain.ReprojectRun{
		ReprojectRunID:    runID,
		Status:            domain.ReprojectStatusSwappable,
		CheckpointPayload: json.RawMessage(`{"last_event_seq":500}`),
		StatsJSON:         json.RawMessage(`{"events_processed":500,"events_total":500}`),
		DiffSummaryJSON:   json.RawMessage(`{"from_item_count":100,"to_item_count":105}`),
		StartedAt:         &startedAt,
		FinishedAt:        &now,
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE knowledge_reproject_runs SET
		status = $2, checkpoint_payload = $3, stats_json = $4, diff_summary_json = $5,
		started_at = $6, finished_at = $7
		WHERE reproject_run_id = $1`)).
		WithArgs(
			runID,
			domain.ReprojectStatusSwappable,
			`{"last_event_seq":500}`,
			`{"events_processed":500,"events_total":500}`,
			`{"from_item_count":100,"to_item_count":105}`,
			&startedAt, &now,
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = repo.UpdateReprojectRun(context.Background(), run)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_UpdateReprojectRun_EmptyJSONFields_DefaultToEmptyObjects(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()

	run := &domain.ReprojectRun{
		ReprojectRunID: runID,
		Status:         domain.ReprojectStatusPending,
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE knowledge_reproject_runs SET
		status = $2, checkpoint_payload = $3, stats_json = $4, diff_summary_json = $5,
		started_at = $6, finished_at = $7
		WHERE reproject_run_id = $1`)).
		WithArgs(
			runID,
			domain.ReprojectStatusPending,
			`{}`,
			`{}`,
			`{}`,
			(*time.Time)(nil), (*time.Time)(nil),
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = repo.UpdateReprojectRun(context.Background(), run)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_ListReprojectRuns_WithStatusFilter(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows([]string{
		"reproject_run_id", "projection_name", "from_version", "to_version", "initiated_by",
		"mode", "status", "range_start", "range_end", "checkpoint_payload",
		"stats_json", "diff_summary_json", "created_at", "started_at", "finished_at",
	}).AddRow(
		runID, "knowledge_home", "v1", "v2", nil,
		domain.ReprojectModeFull, domain.ReprojectStatusRunning,
		nil, nil, json.RawMessage(`{}`),
		json.RawMessage(`{}`), json.RawMessage(`{}`),
		now, &now, nil,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs WHERE status = $1
		ORDER BY created_at DESC LIMIT $2`)).
		WithArgs(domain.ReprojectStatusRunning, 10).
		WillReturnRows(rows)

	result, err := repo.ListReprojectRuns(context.Background(), domain.ReprojectStatusRunning, 10)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, runID, result[0].ReprojectRunID)
	assert.Equal(t, domain.ReprojectStatusRunning, result[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_ListReprojectRuns_WithoutStatusFilter(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	runID1 := uuid.New()
	runID2 := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows([]string{
		"reproject_run_id", "projection_name", "from_version", "to_version", "initiated_by",
		"mode", "status", "range_start", "range_end", "checkpoint_payload",
		"stats_json", "diff_summary_json", "created_at", "started_at", "finished_at",
	}).
		AddRow(
			runID1, "knowledge_home", "v2", "v3", nil,
			domain.ReprojectModeFull, domain.ReprojectStatusPending,
			nil, nil, json.RawMessage(`{}`),
			json.RawMessage(`{}`), json.RawMessage(`{}`),
			now, nil, nil,
		).
		AddRow(
			runID2, "knowledge_home", "v1", "v2", nil,
			domain.ReprojectModeFull, domain.ReprojectStatusSwapped,
			nil, nil, json.RawMessage(`{}`),
			json.RawMessage(`{}`), json.RawMessage(`{}`),
			earlier, &earlier, &now,
		)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT reproject_run_id, projection_name, from_version, to_version, initiated_by,
		mode, status, range_start, range_end, checkpoint_payload,
		stats_json, diff_summary_json, created_at, started_at, finished_at
		FROM knowledge_reproject_runs
		ORDER BY created_at DESC LIMIT $1`)).
		WithArgs(20).
		WillReturnRows(rows)

	result, err := repo.ListReprojectRuns(context.Background(), "", 20)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, runID1, result[0].ReprojectRunID)
	assert.Equal(t, runID2, result[1].ReprojectRunID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_CompareProjections_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	// Mock "from" version stats
	fromStatsRows := pgxmock.NewRows([]string{
		"item_count", "avg_score", "empty_count",
	}).AddRow(int64(100), 0.75, int64(5))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) AS item_count,
		COALESCE(AVG(score), 0) AS avg_score,
		COUNT(*) FILTER (WHERE summary_excerpt = '' OR summary_excerpt IS NULL) AS empty_count
		FROM knowledge_home_items WHERE projection_version = $1`)).
		WithArgs(1).
		WillReturnRows(fromStatsRows)

	// Mock "from" why distribution
	fromWhyRows := pgxmock.NewRows([]string{"reason_code", "count"}).
		AddRow("new_unread", int64(60)).
		AddRow("summary_completed", int64(40))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT r.value->>'code' AS reason_code, COUNT(*) AS count
		FROM knowledge_home_items i, jsonb_array_elements(i.why_json) AS r(value)
		WHERE i.projection_version = $1
		GROUP BY reason_code`)).
		WithArgs(1).
		WillReturnRows(fromWhyRows)

	// Mock "to" version stats
	toStatsRows := pgxmock.NewRows([]string{
		"item_count", "avg_score", "empty_count",
	}).AddRow(int64(105), 0.80, int64(3))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) AS item_count,
		COALESCE(AVG(score), 0) AS avg_score,
		COUNT(*) FILTER (WHERE summary_excerpt = '' OR summary_excerpt IS NULL) AS empty_count
		FROM knowledge_home_items WHERE projection_version = $1`)).
		WithArgs(2).
		WillReturnRows(toStatsRows)

	// Mock "to" why distribution
	toWhyRows := pgxmock.NewRows([]string{"reason_code", "count"}).
		AddRow("new_unread", int64(55)).
		AddRow("summary_completed", int64(50))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT r.value->>'code' AS reason_code, COUNT(*) AS count
		FROM knowledge_home_items i, jsonb_array_elements(i.why_json) AS r(value)
		WHERE i.projection_version = $1
		GROUP BY reason_code`)).
		WithArgs(2).
		WillReturnRows(toWhyRows)

	result, err := repo.CompareProjections(context.Background(), "v1", "v2")
	require.NoError(t, err)
	assert.Equal(t, int64(100), result.FromItemCount)
	assert.Equal(t, int64(105), result.ToItemCount)
	assert.Equal(t, int64(5), result.FromEmptyCount)
	assert.Equal(t, int64(3), result.ToEmptyCount)
	assert.InDelta(t, 0.75, result.FromAvgScore, 0.001)
	assert.InDelta(t, 0.80, result.ToAvgScore, 0.001)
	assert.Equal(t, int64(60), result.FromWhyDistribution["new_unread"])
	assert.Equal(t, int64(40), result.FromWhyDistribution["summary_completed"])
	assert.Equal(t, int64(55), result.ToWhyDistribution["new_unread"])
	assert.Equal(t, int64(50), result.ToWhyDistribution["summary_completed"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_CompareProjections_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) AS item_count,
		COALESCE(AVG(score), 0) AS avg_score,
		COUNT(*) FILTER (WHERE summary_excerpt = '' OR summary_excerpt IS NULL) AS empty_count
		FROM knowledge_home_items WHERE projection_version = $1`)).
		WithArgs(1).
		WillReturnError(fmt.Errorf("connection error"))

	result, err := repo.CompareProjections(context.Background(), "v1", "v2")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "CompareProjections")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_CreateProjectionAudit_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	auditID := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)

	audit := &domain.ProjectionAudit{
		AuditID:           auditID,
		ProjectionName:    "knowledge_home",
		ProjectionVersion: "v2",
		CheckedAt:         now,
		SampleSize:        100,
		MismatchCount:     2,
		DetailsJSON:       json.RawMessage(`{"mismatches":[{"item_key":"a:1"},{"item_key":"a:2"}]}`),
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO knowledge_projection_audits
		(audit_id, projection_name, projection_version, checked_at,
		 sample_size, mismatch_count, details_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`)).
		WithArgs(
			auditID, "knowledge_home", "v2", now,
			100, 2,
			`{"mismatches":[{"item_key":"a:1"},{"item_key":"a:2"}]}`,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = repo.CreateProjectionAudit(context.Background(), audit)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAltDBRepository_ListProjectionAudits_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &AltDBRepository{pool: mock}

	auditID1 := uuid.New()
	auditID2 := uuid.New()
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	rows := pgxmock.NewRows([]string{
		"audit_id", "projection_name", "projection_version", "checked_at",
		"sample_size", "mismatch_count", "details_json",
	}).
		AddRow(auditID1, "knowledge_home", "v2", now, 100, 0, json.RawMessage(`{}`)).
		AddRow(auditID2, "knowledge_home", "v1", earlier, 50, 3, json.RawMessage(`{"mismatches":[]}`))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT audit_id, projection_name, projection_version, checked_at,
		sample_size, mismatch_count, details_json
		FROM knowledge_projection_audits
		WHERE projection_name = $1
		ORDER BY checked_at DESC LIMIT $2`)).
		WithArgs("knowledge_home", 10).
		WillReturnRows(rows)

	result, err := repo.ListProjectionAudits(context.Background(), "knowledge_home", 10)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, auditID1, result[0].AuditID)
	assert.Equal(t, "v2", result[0].ProjectionVersion)
	assert.Equal(t, 100, result[0].SampleSize)
	assert.Equal(t, 0, result[0].MismatchCount)
	assert.Equal(t, auditID2, result[1].AuditID)
	assert.Equal(t, 3, result[1].MismatchCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
