package cutover_readiness_port

import (
	"alt/domain"
	"context"
	"testing"
)

func TestWritePathAuditPortCompiles(t *testing.T) {
	var _ WritePathAuditPort = &mockWritePathAudit{}
	_ = t
}

func TestReconciliationHistoryPortCompiles(t *testing.T) {
	var _ ReconciliationHistoryPort = &mockReconHistory{}
	_ = t
}

func TestReplayHistoryPortCompiles(t *testing.T) {
	var _ ReplayHistoryPort = &mockReplayHistory{}
	_ = t
}

type mockWritePathAudit struct{}

func (m *mockWritePathAudit) CountTotalWritePaths(_ context.Context) (int, error)        { return 14, nil }
func (m *mockWritePathAudit) CountConsolidatedWritePaths(_ context.Context) (int, error)  { return 14, nil }

type mockReconHistory struct{}

func (m *mockReconHistory) GetLatestReconciliation(_ context.Context) (*domain.ReconciliationResult, error) {
	return &domain.ReconciliationResult{Healthy: true}, nil
}
func (m *mockReconHistory) CountConsecutiveHealthy(_ context.Context) (int, error) { return 10, nil }

type mockReplayHistory struct{}

func (m *mockReplayHistory) GetLatestReplayResult(_ context.Context) (*domain.ReprojectRun, error) {
	return &domain.ReprojectRun{Status: domain.ReprojectStatusSwappable}, nil
}
