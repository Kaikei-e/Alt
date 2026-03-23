package domain

import "time"

// ReconciliationResult holds the outcome of a sovereign reconciliation check.
type ReconciliationResult struct {
	ProjectionName string              `json:"projection_name"`
	ActiveVersion  int                 `json:"active_version"`
	CompareVersion int                 `json:"compare_version"`
	DiffSummary    ReprojectDiffSummary `json:"diff_summary"`
	MismatchCount  int                 `json:"mismatch_count"`
	Healthy        bool                `json:"healthy"`
	CheckedAt      time.Time           `json:"checked_at"`
}

// ReconciliationThresholds defines acceptable drift limits.
type ReconciliationThresholds struct {
	MaxItemCountDriftPct float64 `json:"max_item_count_drift_pct"`
	MaxScoreDriftPct     float64 `json:"max_score_drift_pct"`
	MaxMismatchSamples   int     `json:"max_mismatch_samples"`
}

// DefaultReconciliationThresholds returns the default thresholds.
func DefaultReconciliationThresholds() ReconciliationThresholds {
	return ReconciliationThresholds{
		MaxItemCountDriftPct: 0.05,
		MaxScoreDriftPct:     0.1,
		MaxMismatchSamples:   10,
	}
}

// RollbackPrecondition captures whether rollback conditions are met.
type RollbackPrecondition struct {
	CurrentVersion   int     `json:"current_version"`
	TargetVersion    int     `json:"target_version"`
	ReconciliationOK bool    `json:"reconciliation_ok"`
	MismatchPct      float64 `json:"mismatch_pct"`
}

// IsSatisfied returns true if the precondition allows rollback.
func (p RollbackPrecondition) IsSatisfied() bool {
	return p.ReconciliationOK && p.MismatchPct < 0.1
}
