package domain

import "time"

// Service quality mode constants (3-tier degraded mode).
const (
	ServiceQualityFull     = "full"
	ServiceQualityDegraded = "degraded"
	ServiceQualityFallback = "fallback"
)

// SLI name constants.
const (
	SLIAvailability     = "availability"
	SLIFreshness        = "freshness"
	SLIActionDurability = "action_durability"
	SLIStreamContinuity = "stream_continuity"
	SLICorrectnessProxy = "correctness_proxy"
)

// SLO health constants.
const (
	SLOHealthHealthy   = "healthy"
	SLOHealthAtRisk    = "at_risk"
	SLOHealthBreaching = "breaching"
)

// SLI status constants.
const (
	SLIStatusMeeting  = "meeting"
	SLIStatusBurning  = "burning"
	SLIStatusBreached = "breached"
)

// SLOStatus represents the overall SLO health status.
type SLOStatus struct {
	OverallHealth         string         `json:"overall_health"`
	SLIs                  []SLIResult    `json:"slis"`
	ErrorBudgetWindowDays int            `json:"error_budget_window_days"`
	ActiveAlerts          []AlertSummary `json:"active_alerts"`
	ComputedAt            time.Time      `json:"computed_at"`
}

// SLIResult represents the current status of a single SLI.
type SLIResult struct {
	Name                   string  `json:"name"`
	CurrentValue           float64 `json:"current_value"`
	TargetValue            float64 `json:"target_value"`
	Unit                   string  `json:"unit"`
	Status                 string  `json:"status"`
	ErrorBudgetConsumedPct float64 `json:"error_budget_consumed_pct"`
}

// AlertSummary represents an active alert.
type AlertSummary struct {
	AlertName   string    `json:"alert_name"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	FiredAt     time.Time `json:"fired_at"`
	Description string    `json:"description"`
}

// DetermineServiceQuality determines the service quality tier based on projection lag and error.
func DetermineServiceQuality(projectionLag time.Duration, err error) string {
	if err != nil {
		return ServiceQualityFallback
	}
	if projectionLag > 5*time.Minute {
		return ServiceQualityDegraded
	}
	return ServiceQualityFull
}
