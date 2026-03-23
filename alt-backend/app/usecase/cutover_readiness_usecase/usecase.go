package cutover_readiness_usecase

import (
	"alt/domain"
	"alt/port/cutover_readiness_port"
	"context"
	"fmt"
	"time"
)

// Usecase evaluates quantitative cutover readiness for Knowledge Sovereign extraction.
type Usecase struct {
	writePathPort cutover_readiness_port.WritePathAuditPort
	reconPort     cutover_readiness_port.ReconciliationHistoryPort
	replayPort    cutover_readiness_port.ReplayHistoryPort
}

// NewCutoverReadinessUsecase creates a new cutover readiness usecase.
func NewCutoverReadinessUsecase(
	writePathPort cutover_readiness_port.WritePathAuditPort,
	reconPort cutover_readiness_port.ReconciliationHistoryPort,
	replayPort cutover_readiness_port.ReplayHistoryPort,
) *Usecase {
	return &Usecase{
		writePathPort: writePathPort,
		reconPort:     reconPort,
		replayPort:    replayPort,
	}
}

// Execute evaluates all cutover gates and returns a structured readiness report.
func (u *Usecase) Execute(ctx context.Context) (*domain.CutoverReadinessReport, error) {
	gates := domain.DefaultCutoverGates()
	report := &domain.CutoverReadinessReport{
		CheckedAt: time.Now(),
	}

	// Write path consolidation
	wpStatus, err := u.evaluateWritePath(ctx, gates)
	if err != nil {
		return nil, fmt.Errorf("evaluate write path: %w", err)
	}
	report.WritePathConsolidation = wpStatus

	// Reconciliation health
	reconStatus, err := u.evaluateReconciliation(ctx, gates)
	if err != nil {
		return nil, fmt.Errorf("evaluate reconciliation: %w", err)
	}
	report.ReconciliationHealth = reconStatus

	// Replay health
	replayStatus, err := u.evaluateReplay(ctx, gates)
	if err != nil {
		return nil, fmt.Errorf("evaluate replay: %w", err)
	}
	report.ReplayHealth = replayStatus

	// Observability (static check — metrics count is known at compile time)
	report.ObservabilityHealth = domain.ObservabilityStatus{
		MetricsRegistered: 8,
		Ready:             true,
	}

	// Overall readiness
	var blocking []string
	if !wpStatus.Ready {
		blocking = append(blocking, fmt.Sprintf("write path consolidation %.0f%% below 100%%", wpStatus.ConsolidationPct*100))
	}
	if !reconStatus.Ready {
		blocking = append(blocking, fmt.Sprintf("reconciliation: consecutive_ok=%d < %d required", reconStatus.ConsecutiveOK, gates.MinConsecutiveReconOK))
	}
	if !replayStatus.Ready {
		blocking = append(blocking, "replay: no successful replay recorded")
	}

	report.BlockingReasons = blocking
	report.OverallReady = len(blocking) == 0

	return report, nil
}

func (u *Usecase) evaluateWritePath(ctx context.Context, gates domain.CutoverGates) (domain.WritePathStatus, error) {
	total, err := u.writePathPort.CountTotalWritePaths(ctx)
	if err != nil {
		return domain.WritePathStatus{}, fmt.Errorf("count total write paths: %w", err)
	}
	consolidated, err := u.writePathPort.CountConsolidatedWritePaths(ctx)
	if err != nil {
		return domain.WritePathStatus{}, fmt.Errorf("count consolidated write paths: %w", err)
	}

	pct := 0.0
	if total > 0 {
		pct = float64(consolidated) / float64(total)
	}

	return domain.WritePathStatus{
		TotalWritePaths:   total,
		ConsolidatedPaths: consolidated,
		ConsolidationPct:  pct,
		Ready:             pct >= gates.MinConsolidationPct,
	}, nil
}

func (u *Usecase) evaluateReconciliation(ctx context.Context, gates domain.CutoverGates) (domain.ReconciliationStatus, error) {
	latest, err := u.reconPort.GetLatestReconciliation(ctx)
	if err != nil {
		return domain.ReconciliationStatus{}, fmt.Errorf("get latest reconciliation: %w", err)
	}

	consecutiveOK, err := u.reconPort.CountConsecutiveHealthy(ctx)
	if err != nil {
		return domain.ReconciliationStatus{}, fmt.Errorf("count consecutive healthy: %w", err)
	}

	status := domain.ReconciliationStatus{
		ConsecutiveOK: consecutiveOK,
	}

	if latest != nil {
		status.LastCheckAt = &latest.CheckedAt
		if latest.DiffSummary.FromItemCount > 0 {
			status.MismatchRate = float64(latest.MismatchCount) / float64(latest.DiffSummary.FromItemCount)
		}
	}

	status.Ready = consecutiveOK >= gates.MinConsecutiveReconOK && status.MismatchRate <= gates.MaxMismatchRate

	return status, nil
}

func (u *Usecase) evaluateReplay(ctx context.Context, gates domain.CutoverGates) (domain.ReplayStatus, error) {
	latest, err := u.replayPort.GetLatestReplayResult(ctx)
	if err != nil {
		return domain.ReplayStatus{}, fmt.Errorf("get latest replay result: %w", err)
	}

	status := domain.ReplayStatus{}
	if latest != nil {
		status.LastReplayAt = latest.FinishedAt
		status.LastReplayOK = latest.Status == domain.ReprojectStatusSwappable || latest.Status == domain.ReprojectStatusSwapped
	}

	status.Ready = !gates.RequireReplaySuccess || status.LastReplayOK

	return status, nil
}
