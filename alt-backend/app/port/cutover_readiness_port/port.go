package cutover_readiness_port

import (
	"alt/domain"
	"context"
)

// WritePathAuditPort provides write path consolidation counts.
type WritePathAuditPort interface {
	CountTotalWritePaths(ctx context.Context) (int, error)
	CountConsolidatedWritePaths(ctx context.Context) (int, error)
}

// ReconciliationHistoryPort provides reconciliation health history.
type ReconciliationHistoryPort interface {
	GetLatestReconciliation(ctx context.Context) (*domain.ReconciliationResult, error)
	CountConsecutiveHealthy(ctx context.Context) (int, error)
}

// ReplayHistoryPort provides replay/reproject health history.
type ReplayHistoryPort interface {
	GetLatestReplayResult(ctx context.Context) (*domain.ReprojectRun, error)
}
