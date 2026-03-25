package knowledge_projection_health_usecase

import (
	"alt/domain"
	"alt/port/knowledge_backfill_port"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_projection_version_port"
	"alt/port/today_digest_port"
	"alt/utils/logger"
	"context"
	"time"
)

const projectorName = "knowledge-home-projector"

// HealthStatus aggregates projection health information.
type HealthStatus struct {
	ActiveVersion int                            `json:"active_version"`
	CheckpointSeq int64                          `json:"checkpoint_seq"`
	LastUpdated   time.Time                      `json:"last_updated"`
	BackfillJobs  []domain.KnowledgeBackfillJob  `json:"backfill_jobs"`
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
	health := &HealthStatus{}

	// Get active version (best-effort)
	version, err := u.versionPort.GetActiveVersion(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get active version", "error", err)
	} else if version != nil {
		health.ActiveVersion = version.Version
	}

	// Get checkpoint (best-effort)
	seq, err := u.checkpointPort.GetProjectionCheckpoint(ctx, projectorName)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get projection checkpoint", "error", err)
	} else {
		health.CheckpointSeq = seq
	}

	// Get backfill jobs (best-effort)
	jobs, err := u.backfillPort.ListBackfillJobs(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to list backfill jobs", "error", err)
	} else {
		health.BackfillJobs = jobs
	}

	// Use actual checkpoint updated_at instead of request time
	if u.freshnessPort != nil {
		updatedAt, err := u.freshnessPort.GetProjectionFreshness(ctx, projectorName)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to get projection freshness", "error", err)
		} else if updatedAt != nil {
			health.LastUpdated = *updatedAt
			return health, nil
		}
	}
	health.LastUpdated = time.Now()
	return health, nil
}
