package knowledge_sovereign_port

import (
	"alt/domain"
	"context"
)

// ReconciliationReporter persists reconciliation results.
type ReconciliationReporter interface {
	RecordReconciliation(ctx context.Context, result domain.ReconciliationResult) error
}
