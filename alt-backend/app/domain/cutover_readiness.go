package domain

import "time"

// CutoverReadinessReport holds the quantitative readiness assessment for Phase 4 cutover.
type CutoverReadinessReport struct {
	CheckedAt              time.Time           `json:"checked_at"`
	WritePathConsolidation WritePathStatus     `json:"write_path_consolidation"`
	ReconciliationHealth   ReconciliationStatus `json:"reconciliation_health"`
	ReplayHealth           ReplayStatus        `json:"replay_health"`
	ObservabilityHealth    ObservabilityStatus `json:"observability_health"`
	OverallReady           bool                `json:"overall_ready"`
	BlockingReasons        []string            `json:"blocking_reasons,omitempty"`
}

// WritePathStatus tracks write path consolidation progress.
type WritePathStatus struct {
	TotalWritePaths   int     `json:"total_write_paths"`
	ConsolidatedPaths int     `json:"consolidated_paths"`
	ConsolidationPct  float64 `json:"consolidation_pct"`
	Ready             bool    `json:"ready"`
}

// ReconciliationStatus tracks reconciliation health.
type ReconciliationStatus struct {
	LastCheckAt   *time.Time `json:"last_check_at,omitempty"`
	MismatchRate  float64    `json:"mismatch_rate"`
	ConsecutiveOK int        `json:"consecutive_ok"`
	Ready         bool       `json:"ready"`
}

// ReplayStatus tracks replay/reproject health.
type ReplayStatus struct {
	LastReplayAt *time.Time `json:"last_replay_at,omitempty"`
	LastReplayOK bool       `json:"last_replay_ok"`
	Ready        bool       `json:"ready"`
}

// ObservabilityStatus tracks observability readiness.
type ObservabilityStatus struct {
	MetricsRegistered int  `json:"metrics_registered"`
	Ready             bool `json:"ready"`
}

// CutoverGates defines the quantitative thresholds for cutover readiness.
type CutoverGates struct {
	MinConsolidationPct   float64 `json:"min_consolidation_pct"`
	MaxMismatchRate       float64 `json:"max_mismatch_rate"`
	MinConsecutiveReconOK int     `json:"min_consecutive_recon_ok"`
	RequireReplaySuccess  bool    `json:"require_replay_success"`
}

// DefaultCutoverGates returns the default quantitative gates for cutover readiness.
func DefaultCutoverGates() CutoverGates {
	return CutoverGates{
		MinConsolidationPct:   1.0,
		MaxMismatchRate:       0.01,
		MinConsecutiveReconOK: 10,
		RequireReplaySuccess:  true,
	}
}
