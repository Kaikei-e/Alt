package cutover_readiness_usecase

import (
	"alt/domain"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWritePathAudit struct {
	total        int
	consolidated int
}

func (m *mockWritePathAudit) CountTotalWritePaths(_ context.Context) (int, error)       { return m.total, nil }
func (m *mockWritePathAudit) CountConsolidatedWritePaths(_ context.Context) (int, error) { return m.consolidated, nil }

type mockReconHistory struct {
	latest        *domain.ReconciliationResult
	consecutiveOK int
}

func (m *mockReconHistory) GetLatestReconciliation(_ context.Context) (*domain.ReconciliationResult, error) {
	return m.latest, nil
}
func (m *mockReconHistory) CountConsecutiveHealthy(_ context.Context) (int, error) {
	return m.consecutiveOK, nil
}

type mockReplayHistory struct {
	latest *domain.ReprojectRun
}

func (m *mockReplayHistory) GetLatestReplayResult(_ context.Context) (*domain.ReprojectRun, error) {
	return m.latest, nil
}

func TestCutoverReadiness_AllGatesMet(t *testing.T) {
	now := time.Now()
	uc := NewCutoverReadinessUsecase(
		&mockWritePathAudit{total: 14, consolidated: 14},
		&mockReconHistory{
			latest:        &domain.ReconciliationResult{Healthy: true, CheckedAt: now},
			consecutiveOK: 15,
		},
		&mockReplayHistory{latest: &domain.ReprojectRun{Status: domain.ReprojectStatusSwappable, FinishedAt: &now}},
	)
	report, err := uc.Execute(context.Background())
	require.NoError(t, err)
	assert.True(t, report.OverallReady)
	assert.Empty(t, report.BlockingReasons)
	assert.True(t, report.WritePathConsolidation.Ready)
	assert.True(t, report.ReconciliationHealth.Ready)
	assert.True(t, report.ReplayHealth.Ready)
	assert.True(t, report.ObservabilityHealth.Ready)
}

func TestCutoverReadiness_WritePathIncomplete(t *testing.T) {
	now := time.Now()
	uc := NewCutoverReadinessUsecase(
		&mockWritePathAudit{total: 14, consolidated: 10},
		&mockReconHistory{
			latest:        &domain.ReconciliationResult{Healthy: true, CheckedAt: now},
			consecutiveOK: 15,
		},
		&mockReplayHistory{latest: &domain.ReprojectRun{Status: domain.ReprojectStatusSwappable, FinishedAt: &now}},
	)
	report, err := uc.Execute(context.Background())
	require.NoError(t, err)
	assert.False(t, report.OverallReady)
	assert.Contains(t, report.BlockingReasons[0], "consolidation")
}

func TestCutoverReadiness_ReconciliationUnhealthy(t *testing.T) {
	now := time.Now()
	uc := NewCutoverReadinessUsecase(
		&mockWritePathAudit{total: 14, consolidated: 14},
		&mockReconHistory{
			latest:        &domain.ReconciliationResult{Healthy: false, MismatchCount: 3, CheckedAt: now},
			consecutiveOK: 2,
		},
		&mockReplayHistory{latest: &domain.ReprojectRun{Status: domain.ReprojectStatusSwappable, FinishedAt: &now}},
	)
	report, err := uc.Execute(context.Background())
	require.NoError(t, err)
	assert.False(t, report.OverallReady)
	assert.False(t, report.ReconciliationHealth.Ready)
}

func TestCutoverReadiness_NoReplayHistory(t *testing.T) {
	now := time.Now()
	uc := NewCutoverReadinessUsecase(
		&mockWritePathAudit{total: 14, consolidated: 14},
		&mockReconHistory{
			latest:        &domain.ReconciliationResult{Healthy: true, CheckedAt: now},
			consecutiveOK: 15,
		},
		&mockReplayHistory{latest: nil},
	)
	report, err := uc.Execute(context.Background())
	require.NoError(t, err)
	assert.False(t, report.OverallReady)
	assert.Contains(t, report.BlockingReasons[0], "replay")
}
