package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReconciliationResult_Fields(t *testing.T) {
	r := ReconciliationResult{
		ProjectionName: "knowledge_home",
		ActiveVersion:  2,
		CompareVersion: 1,
		DiffSummary:    ReprojectDiffSummary{FromItemCount: 100, ToItemCount: 98},
		MismatchCount:  2,
		Healthy:        true,
	}
	assert.Equal(t, "knowledge_home", r.ProjectionName)
	assert.Equal(t, 2, r.ActiveVersion)
	assert.True(t, r.Healthy)
}

func TestDefaultReconciliationThresholds(t *testing.T) {
	th := DefaultReconciliationThresholds()
	assert.Equal(t, 0.05, th.MaxItemCountDriftPct)
	assert.Equal(t, 0.1, th.MaxScoreDriftPct)
	assert.Equal(t, 10, th.MaxMismatchSamples)
}

func TestRollbackPrecondition_Satisfied(t *testing.T) {
	p := RollbackPrecondition{
		CurrentVersion:   2,
		TargetVersion:    1,
		ReconciliationOK: true,
		MismatchPct:      0.02,
	}
	assert.True(t, p.IsSatisfied())
}

func TestRollbackPrecondition_NotSatisfied_HighMismatch(t *testing.T) {
	p := RollbackPrecondition{
		CurrentVersion:   2,
		TargetVersion:    1,
		ReconciliationOK: false,
		MismatchPct:      0.15,
	}
	assert.False(t, p.IsSatisfied())
}

func TestRollbackPrecondition_NotSatisfied_ReconciliationFailed(t *testing.T) {
	p := RollbackPrecondition{
		CurrentVersion:   2,
		TargetVersion:    1,
		ReconciliationOK: false,
		MismatchPct:      0.01,
	}
	assert.False(t, p.IsSatisfied())
}
