package knowledge_reproject_usecase

import (
	"alt/domain"
	"alt/port/knowledge_projection_port"
	"alt/port/knowledge_projection_version_port"
	"alt/port/knowledge_reproject_port"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// projectorName is the checkpoint key used by the knowledge home projector.
const projectorName = "knowledge-home-projector"

// Usecase orchestrates reproject run lifecycle.
type Usecase struct {
	createRunPort        knowledge_reproject_port.CreateReprojectRunPort
	getRunPort           knowledge_reproject_port.GetReprojectRunPort
	updateRunPort        knowledge_reproject_port.UpdateReprojectRunPort
	listRunsPort         knowledge_reproject_port.ListReprojectRunsPort
	comparePort          knowledge_reproject_port.CompareProjectionsPort
	activeVersionPort    knowledge_projection_version_port.GetActiveVersionPort
	activateVersionPort  knowledge_projection_version_port.ActivateVersionPort
	createVersionPort    knowledge_projection_version_port.CreateVersionPort
	updateCheckpointPort knowledge_projection_port.UpdateProjectionCheckpointPort
}

// NewUsecase creates a new reproject usecase.
func NewUsecase(
	createRunPort knowledge_reproject_port.CreateReprojectRunPort,
	getRunPort knowledge_reproject_port.GetReprojectRunPort,
	updateRunPort knowledge_reproject_port.UpdateReprojectRunPort,
	listRunsPort knowledge_reproject_port.ListReprojectRunsPort,
	comparePort knowledge_reproject_port.CompareProjectionsPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	activateVersionPort knowledge_projection_version_port.ActivateVersionPort,
	createVersionPort ...knowledge_projection_version_port.CreateVersionPort,
) *Usecase {
	uc := &Usecase{
		createRunPort:       createRunPort,
		getRunPort:          getRunPort,
		updateRunPort:       updateRunPort,
		listRunsPort:        listRunsPort,
		comparePort:         comparePort,
		activeVersionPort:   activeVersionPort,
		activateVersionPort: activateVersionPort,
	}
	if len(createVersionPort) > 0 {
		uc.createVersionPort = createVersionPort[0]
	}
	return uc
}

// WithUpdateCheckpointPort sets the checkpoint port used to reset the projector
// checkpoint when swapping projection versions.
func (u *Usecase) WithUpdateCheckpointPort(port knowledge_projection_port.UpdateProjectionCheckpointPort) *Usecase {
	u.updateCheckpointPort = port
	return u
}

// StartReproject validates the mode and creates a pending reproject run.
func (u *Usecase) StartReproject(ctx context.Context, mode, fromVersion, toVersion string, rangeStart, rangeEnd *time.Time) (*domain.ReprojectRun, error) {
	if !domain.IsValidReprojectMode(mode) {
		return nil, fmt.Errorf("invalid reproject mode %q", mode)
	}

	if mode == domain.ReprojectModeTimeRange {
		if rangeStart == nil || rangeEnd == nil {
			return nil, fmt.Errorf("range_start and range_end are required for time_range mode")
		}
	}

	now := time.Now()
	run := &domain.ReprojectRun{
		ReprojectRunID: uuid.New(),
		ProjectionName: "knowledge_home",
		FromVersion:    fromVersion,
		ToVersion:      toVersion,
		Mode:           mode,
		Status:         domain.ReprojectStatusPending,
		RangeStart:     rangeStart,
		RangeEnd:       rangeEnd,
		CreatedAt:      now,
	}

	// Ensure target version exists in knowledge_projection_versions
	if u.createVersionPort != nil {
		targetVersionNum, parseErr := strconv.Atoi(strings.TrimPrefix(strings.ToLower(toVersion), "v"))
		if parseErr == nil {
			_ = u.createVersionPort.CreateVersion(ctx, domain.KnowledgeProjectionVersion{
				Version:     targetVersionNum,
				Description: fmt.Sprintf("V%d reproject from %s", targetVersionNum, fromVersion),
				Status:      "inactive",
				CreatedAt:   now,
				ActivatedAt: &now,
			})
			// Ignore duplicate key errors — version may already exist
		}
	}

	if err := u.createRunPort.CreateReprojectRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create reproject run: %w", err)
	}

	return run, nil
}

// GetReprojectStatus returns the current status of a reproject run.
func (u *Usecase) GetReprojectStatus(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error) {
	run, err := u.getRunPort.GetReprojectRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("get reproject status: %w", err)
	}
	return run, nil
}

// ListReprojectRuns returns reproject runs with optional status filter.
func (u *Usecase) ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error) {
	runs, err := u.listRunsPort.ListReprojectRuns(ctx, statusFilter, limit)
	if err != nil {
		return nil, fmt.Errorf("list reproject runs: %w", err)
	}
	return runs, nil
}

// CompareReproject validates that the run is in validating or swappable status, runs the comparison,
// and updates the run to swappable with the diff summary.
func (u *Usecase) CompareReproject(ctx context.Context, runID uuid.UUID) (*domain.ReprojectDiffSummary, error) {
	run, err := u.getRunPort.GetReprojectRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("compare reproject get run: %w", err)
	}

	if run.Status != domain.ReprojectStatusValidating && run.Status != domain.ReprojectStatusSwappable {
		return nil, fmt.Errorf("cannot compare run in status %q; must be validating or swappable", run.Status)
	}

	diff, err := u.comparePort.CompareProjections(ctx, run.FromVersion, run.ToVersion)
	if err != nil {
		return nil, fmt.Errorf("compare projections: %w", err)
	}

	diffJSON, err := json.Marshal(diff)
	if err != nil {
		return nil, fmt.Errorf("marshal diff summary: %w", err)
	}

	run.DiffSummaryJSON = diffJSON
	run.Status = domain.ReprojectStatusSwappable

	if err := u.updateRunPort.UpdateReprojectRun(ctx, run); err != nil {
		return nil, fmt.Errorf("update reproject run after compare: %w", err)
	}

	return diff, nil
}

// SwapReproject validates the run is swappable, activates the new version,
// resets the projector checkpoint to the reproject's final event_seq, and marks
// the run as swapped.
func (u *Usecase) SwapReproject(ctx context.Context, runID uuid.UUID) error {
	run, err := u.getRunPort.GetReprojectRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("swap reproject get run: %w", err)
	}

	if run.Status != domain.ReprojectStatusSwappable {
		return fmt.Errorf("cannot swap run in status %q; must be swappable", run.Status)
	}

	version, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(run.ToVersion), "v"))
	if err != nil {
		return fmt.Errorf("parse version for activation %q: %w", run.ToVersion, err)
	}

	if err := u.activateVersionPort.ActivateVersion(ctx, version); err != nil {
		return fmt.Errorf("activate version %d: %w", version, err)
	}

	// Reset projector checkpoint to the reproject's final event_seq so the live
	// projector replays events that occurred between the reproject snapshot and now.
	if u.updateCheckpointPort != nil {
		if seq := extractCheckpointSeq(run.CheckpointPayload); seq > 0 {
			if err := u.updateCheckpointPort.UpdateProjectionCheckpoint(ctx, projectorName, seq); err != nil {
				slog.ErrorContext(ctx, "failed to reset projector checkpoint after swap",
					"version", version, "target_seq", seq, "error", err)
				return fmt.Errorf("reset checkpoint to %d: %w", seq, err)
			}
			slog.InfoContext(ctx, "projector checkpoint reset after swap",
				"version", version, "checkpoint_seq", seq)
		}
	}

	now := time.Now()
	run.Status = domain.ReprojectStatusSwapped
	run.FinishedAt = &now

	if err := u.updateRunPort.UpdateReprojectRun(ctx, run); err != nil {
		return fmt.Errorf("update reproject run after swap: %w", err)
	}

	return nil
}

// extractCheckpointSeq parses last_event_seq from a reproject run's checkpoint payload.
func extractCheckpointSeq(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var cp struct {
		LastEventSeq int64 `json:"last_event_seq"`
	}
	if err := json.Unmarshal(payload, &cp); err != nil {
		return 0
	}
	return cp.LastEventSeq
}

// RollbackReproject validates the run is swapped, reverts to the previous version via
// ActivateVersion, and marks the run as cancelled.
func (u *Usecase) RollbackReproject(ctx context.Context, runID uuid.UUID) error {
	run, err := u.getRunPort.GetReprojectRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("rollback reproject get run: %w", err)
	}

	if run.Status != domain.ReprojectStatusSwapped {
		return fmt.Errorf("cannot rollback run in status %q; must be swapped", run.Status)
	}

	// Revert to the previous version
	fromVersion, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(run.FromVersion), "v"))
	if err != nil {
		return fmt.Errorf("parse from_version for rollback %q: %w", run.FromVersion, err)
	}

	if err := u.activateVersionPort.ActivateVersion(ctx, fromVersion); err != nil {
		return fmt.Errorf("rollback activate version %d: %w", fromVersion, err)
	}

	run.Status = domain.ReprojectStatusCancelled

	if err := u.updateRunPort.UpdateReprojectRun(ctx, run); err != nil {
		return fmt.Errorf("update reproject run after rollback: %w", err)
	}

	return nil
}
