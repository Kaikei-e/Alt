package knowledge_sovereign_port

import (
	"alt/domain"
	"context"
	"testing"
)

func TestReconciliationReporterInterfaceCompiles(t *testing.T) {
	var _ ReconciliationReporter = &mockReconciliationReporter{}
	_ = t
}

type mockReconciliationReporter struct{}

func (m *mockReconciliationReporter) RecordReconciliation(_ context.Context, _ domain.ReconciliationResult) error {
	return nil
}
