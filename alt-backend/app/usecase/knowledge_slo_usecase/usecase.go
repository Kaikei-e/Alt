package knowledge_slo_usecase

import (
	"alt/domain"
	"alt/port/knowledge_slo_port"
	"alt/utils/logger"
	"context"
	"time"
)

const (
	// freshness SLO target: projection lag must be under 5 minutes (300 seconds).
	freshnessTargetSeconds = 300.0
	// default error budget window in days.
	errorBudgetWindowDays = 30
)

// Usecase provides SLO status aggregation.
type Usecase struct {
	lagPort knowledge_slo_port.GetProjectionLagPort
}

// NewUsecase creates a new SLO usecase.
func NewUsecase(lagPort knowledge_slo_port.GetProjectionLagPort) *Usecase {
	return &Usecase{lagPort: lagPort}
}

// GetSLOStatus aggregates SLI values into an SLO status report.
func (u *Usecase) GetSLOStatus(ctx context.Context) (*domain.SLOStatus, error) {
	status := &domain.SLOStatus{
		ErrorBudgetWindowDays: errorBudgetWindowDays,
		ComputedAt:            time.Now(),
		ActiveAlerts:          []domain.AlertSummary{},
	}

	// SLI-B: Freshness (from DB via port)
	freshnessSLI := u.computeFreshnessSLI(ctx)

	// Placeholder SLIs for metrics not yet wired to Prometheus
	availabilitySLI := domain.SLIResult{
		Name:                   domain.SLIAvailability,
		CurrentValue:           100.0,
		TargetValue:            99.9,
		Unit:                   "percent",
		Status:                 domain.SLIStatusMeeting,
		ErrorBudgetConsumedPct: 0.0,
	}

	actionDurabilitySLI := domain.SLIResult{
		Name:                   domain.SLIActionDurability,
		CurrentValue:           100.0,
		TargetValue:            99.99,
		Unit:                   "percent",
		Status:                 domain.SLIStatusMeeting,
		ErrorBudgetConsumedPct: 0.0,
	}

	streamContinuitySLI := domain.SLIResult{
		Name:                   domain.SLIStreamContinuity,
		CurrentValue:           100.0,
		TargetValue:            99.5,
		Unit:                   "percent",
		Status:                 domain.SLIStatusMeeting,
		ErrorBudgetConsumedPct: 0.0,
	}

	correctnessProxySLI := domain.SLIResult{
		Name:                   domain.SLICorrectnessProxy,
		CurrentValue:           100.0,
		TargetValue:            99.0,
		Unit:                   "percent",
		Status:                 domain.SLIStatusMeeting,
		ErrorBudgetConsumedPct: 0.0,
	}

	status.SLIs = []domain.SLIResult{
		availabilitySLI,
		freshnessSLI,
		actionDurabilitySLI,
		streamContinuitySLI,
		correctnessProxySLI,
	}

	status.OverallHealth = u.computeOverallHealth(status.SLIs)
	return status, nil
}

// computeFreshnessSLI reads projection lag and converts it into an SLI result.
func (u *Usecase) computeFreshnessSLI(ctx context.Context) domain.SLIResult {
	sli := domain.SLIResult{
		Name:        domain.SLIFreshness,
		TargetValue: freshnessTargetSeconds,
		Unit:        "seconds",
	}

	lag, err := u.lagPort.GetProjectionLag(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get projection lag for freshness SLI", "error", err)
		sli.CurrentValue = -1
		sli.Status = domain.SLIStatusBreached
		sli.ErrorBudgetConsumedPct = 100.0
		return sli
	}

	lagSeconds := lag.Seconds()
	sli.CurrentValue = lagSeconds

	if lagSeconds <= freshnessTargetSeconds {
		sli.Status = domain.SLIStatusMeeting
		sli.ErrorBudgetConsumedPct = (lagSeconds / freshnessTargetSeconds) * 100.0
	} else {
		sli.Status = domain.SLIStatusBurning
		sli.ErrorBudgetConsumedPct = 100.0
	}

	return sli
}

// computeOverallHealth determines the overall health from all SLIs.
func (u *Usecase) computeOverallHealth(slis []domain.SLIResult) string {
	hasBreached := false
	hasBurning := false

	for _, sli := range slis {
		switch sli.Status {
		case domain.SLIStatusBreached:
			hasBreached = true
		case domain.SLIStatusBurning:
			hasBurning = true
		}
	}

	if hasBreached {
		return domain.SLOHealthBreaching
	}
	if hasBurning {
		return domain.SLOHealthAtRisk
	}
	return domain.SLOHealthHealthy
}
