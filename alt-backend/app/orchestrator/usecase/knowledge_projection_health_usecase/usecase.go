package knowledge_projection_health_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/knowledge_backfill_port"
	"alt/orchestrator/port/knowledge_projection_port"
	"alt/orchestrator/port/knowledge_projection_version_port"
	"alt/orchestrator/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"time"
)

const projectorName = "knowledge-home-projector"

// HealthStatus aggregates projection health information.
type HealthStatus struct {
	ActiveVersion int                           `json:"active_version"`
	CheckpointSeq int64                         `json:"checkpoint_seq"`
	LastUpdated   time.Time                     `json:"last_updated"`
	BackfillJobs  []domain.KnowledgeBackfillJob `json:"backfill_jobs"`
}

// Usecase provides projection health information.
type Usecase struct {
	versionPort    knowledge_projection_version_port.GetActiveVersionPort
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort
	backfillPort   knowledge_backfill_port.ListBackfillJobsPort
	freshnessPort  today_digest_port.GetProjectionFreshnessPort
}

// NewUsecase creates a new projection health usecase.
func NewUsecase(
	versionPort knowledge_projection_version_port.GetActiveVersionPort,
	checkpointPort knowledge_projection_port.GetProjectionCheckpointPort,
	backfillPort knowledge_backfill_port.ListBackfillJobsPort,
	freshnessPort today_digest_port.GetProjectionFreshnessPort,
) *Usecase {
	return &Usecase{
		versionPort:    versionPort,
		checkpointPort: checkpointPort,
		backfillPort:   backfillPort,
		freshnessPort:  freshnessPort,
	}
}

// GetHealth aggregates projection health data.
func (u *Usecase) GetHealth(ctx context.Context) (*HealthStatus, error) {
	var activeVersion int
	version, err := u.versionPort.GetActiveVersion(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get active version", "error", err)
	} else if version != nil {
		activeVersion = version.Version
	}

	var checkpointSeq int64
	seq, err := u.checkpointPort.GetProjectionCheckpoint(ctx, projectorName)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get projection checkpoint", "error", err)
	} else {
		checkpointSeq = seq
	}

	var backfillJobs []domain.KnowledgeBackfillJob
	jobs, err := u.backfillPort.ListBackfillJobs(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to list backfill jobs", "error", err)
	} else {
		backfillJobs = jobs
	}

	// Use actual checkpoint updated_at instead of request time
	var updatedAt *time.Time
	if u.freshnessPort != nil {
		freshness, err := u.freshnessPort.GetProjectionFreshness(ctx, projectorName)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to get projection freshness", "error", err)
		} else {
			updatedAt = freshness
		}
	}

	lastUpdated := resolveLastUpdated(updatedAt, time.Now())
	return buildHealthStatus(activeVersion, checkpointSeq, backfillJobs, lastUpdated), nil
}

// resolveLastUpdated returns the checkpoint timestamp if available, or the fallback time.
func resolveLastUpdated(updatedAt *time.Time, now time.Time) time.Time {
	if updatedAt != nil {
		return *updatedAt
	}
	return now
}

// buildHealthStatus constructs the aggregated health status struct.
func buildHealthStatus(activeVersion int, seq int64, jobs []domain.KnowledgeBackfillJob, lastUpdated time.Time) *HealthStatus {
	return &HealthStatus{
		ActiveVersion: activeVersion,
		CheckpointSeq: seq,
		BackfillJobs:  jobs,
		LastUpdated:   lastUpdated,
	}
}
