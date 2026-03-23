package job

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComparePort struct {
	diff *domain.ReprojectDiffSummary
	err  error
}

func (m *mockComparePort) CompareProjections(_ context.Context, _, _ string) (*domain.ReprojectDiffSummary, error) {
	return m.diff, m.err
}

// mockActiveVersionPort is defined in knowledge_projector_test.go (same package).

type reconciliationReporterMock struct {
	lastResult *domain.ReconciliationResult
}

func (m *reconciliationReporterMock) RecordReconciliation(_ context.Context, result domain.ReconciliationResult) error {
	m.lastResult = &result
	return nil
}

func TestSovereignReconciliation_NoDrift(t *testing.T) {
	logger.InitLogger()

	compare := &mockComparePort{diff: &domain.ReprojectDiffSummary{
		FromItemCount: 100, ToItemCount: 100,
		FromAvgScore: 0.5, ToAvgScore: 0.5,
	}}
	version := &mockActiveVersionPort{version: &domain.KnowledgeProjectionVersion{Version: 2}}
	reporter := &reconciliationReporterMock{}

	fn := SovereignReconciliationJob(compare, version, reporter, nil)
	err := fn(context.Background())
	require.NoError(t, err)

	require.NotNil(t, reporter.lastResult)
	assert.True(t, reporter.lastResult.Healthy)
	assert.Equal(t, 0, reporter.lastResult.MismatchCount)
	assert.Equal(t, 2, reporter.lastResult.ActiveVersion)
	assert.Equal(t, 1, reporter.lastResult.CompareVersion)
}

func TestSovereignReconciliation_DriftDetected(t *testing.T) {
	logger.InitLogger()

	compare := &mockComparePort{diff: &domain.ReprojectDiffSummary{
		FromItemCount: 100, ToItemCount: 80, // 20% drift
		FromAvgScore: 0.5, ToAvgScore: 0.3,  // 40% score drift
	}}
	version := &mockActiveVersionPort{version: &domain.KnowledgeProjectionVersion{Version: 2}}
	reporter := &reconciliationReporterMock{}

	fn := SovereignReconciliationJob(compare, version, reporter, nil)
	err := fn(context.Background())
	require.NoError(t, err)

	require.NotNil(t, reporter.lastResult)
	assert.False(t, reporter.lastResult.Healthy)
	assert.Greater(t, reporter.lastResult.MismatchCount, 0)
}

func TestSovereignReconciliation_VersionOneSkips(t *testing.T) {
	logger.InitLogger()

	version := &mockActiveVersionPort{version: &domain.KnowledgeProjectionVersion{Version: 1}}

	fn := SovereignReconciliationJob(nil, version, nil, nil)
	err := fn(context.Background())
	require.NoError(t, err) // graceful skip
}

func TestSovereignReconciliation_NilVersionSkips(t *testing.T) {
	logger.InitLogger()

	version := &mockActiveVersionPort{version: nil}

	fn := SovereignReconciliationJob(nil, version, nil, nil)
	err := fn(context.Background())
	require.NoError(t, err)
}
