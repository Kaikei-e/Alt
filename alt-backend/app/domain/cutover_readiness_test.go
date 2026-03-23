package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCutoverReadinessReport_AllReady(t *testing.T) {
	report := CutoverReadinessReport{
		WritePathConsolidation: WritePathStatus{
			TotalWritePaths: 14, ConsolidatedPaths: 14, ConsolidationPct: 1.0, Ready: true,
		},
		ReconciliationHealth: ReconciliationStatus{MismatchRate: 0.0, ConsecutiveOK: 15, Ready: true},
		ReplayHealth:         ReplayStatus{LastReplayOK: true, Ready: true},
		ObservabilityHealth:  ObservabilityStatus{MetricsRegistered: 8, Ready: true},
		OverallReady:         true,
	}
	assert.True(t, report.OverallReady)
	assert.Empty(t, report.BlockingReasons)
}

func TestCutoverReadinessReport_NotReady_LowConsolidation(t *testing.T) {
	report := CutoverReadinessReport{
		WritePathConsolidation: WritePathStatus{
			TotalWritePaths: 14, ConsolidatedPaths: 10, ConsolidationPct: 0.71, Ready: false,
		},
		OverallReady:    false,
		BlockingReasons: []string{"write path consolidation below 100%"},
	}
	assert.False(t, report.OverallReady)
	assert.NotEmpty(t, report.BlockingReasons)
}

func TestDefaultCutoverGates(t *testing.T) {
	gates := DefaultCutoverGates()
	assert.Equal(t, 1.0, gates.MinConsolidationPct)
	assert.Equal(t, 0.01, gates.MaxMismatchRate)
	assert.Equal(t, 10, gates.MinConsecutiveReconOK)
	assert.True(t, gates.RequireReplaySuccess)
}
