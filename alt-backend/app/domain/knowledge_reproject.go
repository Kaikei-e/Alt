package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Reproject run status constants.
const (
	ReprojectStatusPending    = "pending"
	ReprojectStatusRunning    = "running"
	ReprojectStatusValidating = "validating"
	ReprojectStatusSwappable  = "swappable"
	ReprojectStatusSwapped    = "swapped"
	ReprojectStatusFailed     = "failed"
	ReprojectStatusCancelled  = "cancelled"
)

// Reproject mode constants.
const (
	ReprojectModeDryRun     = "dry_run"
	ReprojectModeUserSubset = "user_subset"
	ReprojectModeTimeRange  = "time_range"
	ReprojectModeFull       = "full"
)

// ReprojectRun represents a single projection re-build operation.
type ReprojectRun struct {
	ReprojectRunID    uuid.UUID       `json:"reproject_run_id" db:"reproject_run_id"`
	ProjectionName    string          `json:"projection_name" db:"projection_name"`
	FromVersion       string          `json:"from_version" db:"from_version"`
	ToVersion         string          `json:"to_version" db:"to_version"`
	InitiatedBy       *uuid.UUID      `json:"initiated_by" db:"initiated_by"`
	Mode              string          `json:"mode" db:"mode"`
	Status            string          `json:"status" db:"status"`
	RangeStart        *time.Time      `json:"range_start" db:"range_start"`
	RangeEnd          *time.Time      `json:"range_end" db:"range_end"`
	CheckpointPayload json.RawMessage `json:"checkpoint_payload" db:"checkpoint_payload"`
	StatsJSON         json.RawMessage `json:"stats_json" db:"stats_json"`
	DiffSummaryJSON   json.RawMessage `json:"diff_summary_json" db:"diff_summary_json"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	StartedAt         *time.Time      `json:"started_at" db:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at" db:"finished_at"`
}

// ReprojectDiffSummary contains comparison metrics between two projection versions.
type ReprojectDiffSummary struct {
	FromItemCount       int64            `json:"from_item_count"`
	ToItemCount         int64            `json:"to_item_count"`
	FromEmptyCount      int64            `json:"from_empty_count"`
	ToEmptyCount        int64            `json:"to_empty_count"`
	FromAvgScore        float64          `json:"from_avg_score"`
	ToAvgScore          float64          `json:"to_avg_score"`
	FromWhyDistribution map[string]int64 `json:"from_why_distribution"`
	ToWhyDistribution   map[string]int64 `json:"to_why_distribution"`
}

// ReprojectStats contains progress statistics for a reproject run.
type ReprojectStats struct {
	EventsProcessed int64         `json:"events_processed"`
	EventsTotal     int64         `json:"events_total"`
	ErrorCount      int64         `json:"error_count"`
	Duration        time.Duration `json:"duration_ns"`
}

// ProjectionAudit represents the result of a projection audit check.
type ProjectionAudit struct {
	AuditID           uuid.UUID       `json:"audit_id" db:"audit_id"`
	ProjectionName    string          `json:"projection_name" db:"projection_name"`
	ProjectionVersion string          `json:"projection_version" db:"projection_version"`
	CheckedAt         time.Time       `json:"checked_at" db:"checked_at"`
	SampleSize        int             `json:"sample_size" db:"sample_size"`
	MismatchCount     int             `json:"mismatch_count" db:"mismatch_count"`
	DetailsJSON       json.RawMessage `json:"details_json" db:"details_json"`
}

// ValidReprojectModes returns the set of valid reproject modes.
func ValidReprojectModes() []string {
	return []string{
		ReprojectModeDryRun,
		ReprojectModeUserSubset,
		ReprojectModeTimeRange,
		ReprojectModeFull,
	}
}

// IsValidReprojectMode checks if a mode string is valid.
func IsValidReprojectMode(mode string) bool {
	for _, m := range ValidReprojectModes() {
		if m == mode {
			return true
		}
	}
	return false
}
