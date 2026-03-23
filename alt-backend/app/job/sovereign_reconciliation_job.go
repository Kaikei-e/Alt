package job

import (
	"alt/domain"
	"alt/port/knowledge_projection_version_port"
	"alt/port/knowledge_reproject_port"
	"alt/port/knowledge_sovereign_port"
	altotel "alt/utils/otel"
	"alt/utils/logger"
	"context"
	"fmt"
	"math"
	"time"
)

// SovereignReconciliationJob returns a scheduled job that periodically compares
// the active projection version with the previous version and records drift.
func SovereignReconciliationJob(
	comparePort knowledge_reproject_port.CompareProjectionsPort,
	versionPort knowledge_projection_version_port.GetActiveVersionPort,
	reporter knowledge_sovereign_port.ReconciliationReporter,
	metrics *altotel.KnowledgeHomeMetrics,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return runSovereignReconciliation(ctx, comparePort, versionPort, reporter, metrics)
	}
}

func runSovereignReconciliation(
	ctx context.Context,
	comparePort knowledge_reproject_port.CompareProjectionsPort,
	versionPort knowledge_projection_version_port.GetActiveVersionPort,
	reporter knowledge_sovereign_port.ReconciliationReporter,
	metrics *altotel.KnowledgeHomeMetrics,
) error {
	start := time.Now()

	// Get active version
	version, err := versionPort.GetActiveVersion(ctx)
	if err != nil {
		return fmt.Errorf("sovereign reconciliation: get active version: %w", err)
	}
	if version == nil || version.Version <= 1 {
		// No previous version to compare against
		return nil
	}

	activeVersion := version.Version
	compareVersion := activeVersion - 1

	fromVersionStr := fmt.Sprintf("v%d", compareVersion)
	toVersionStr := fmt.Sprintf("v%d", activeVersion)

	diff, err := comparePort.CompareProjections(ctx, fromVersionStr, toVersionStr)
	if err != nil {
		return fmt.Errorf("sovereign reconciliation: compare projections: %w", err)
	}

	thresholds := domain.DefaultReconciliationThresholds()
	healthy := true
	mismatchCount := 0

	// Check item count drift
	if diff.FromItemCount > 0 {
		drift := math.Abs(float64(diff.ToItemCount-diff.FromItemCount)) / float64(diff.FromItemCount)
		if drift > thresholds.MaxItemCountDriftPct {
			healthy = false
			mismatchCount++
		}
	}

	// Check score drift
	if diff.FromAvgScore > 0 {
		scoreDrift := math.Abs(diff.ToAvgScore-diff.FromAvgScore) / diff.FromAvgScore
		if scoreDrift > thresholds.MaxScoreDriftPct {
			healthy = false
			mismatchCount++
		}
	}

	result := domain.ReconciliationResult{
		ProjectionName: "knowledge_home",
		ActiveVersion:  activeVersion,
		CompareVersion: compareVersion,
		DiffSummary:    *diff,
		MismatchCount:  mismatchCount,
		Healthy:        healthy,
		CheckedAt:      time.Now(),
	}

	if reporter != nil {
		if err := reporter.RecordReconciliation(ctx, result); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to record reconciliation result", "error", err)
		}
	}

	// Record metrics
	if metrics != nil {
		metrics.SovereignReconciliationRun.Add(ctx, 1)
		if mismatchCount > 0 {
			metrics.SovereignReconciliationMismatch.Add(ctx, int64(mismatchCount))
		}
		metrics.SovereignReconciliationDuration.Record(ctx, time.Since(start).Seconds())
		cutoverReady := 0.0
		if healthy {
			cutoverReady = 1.0
		}
		metrics.SovereignCutoverReady.Record(ctx, cutoverReady)
	}

	if !healthy {
		logger.Logger.WarnContext(ctx, "sovereign reconciliation detected drift",
			"active_version", activeVersion,
			"compare_version", compareVersion,
			"mismatch_count", mismatchCount)
	}

	return nil
}
