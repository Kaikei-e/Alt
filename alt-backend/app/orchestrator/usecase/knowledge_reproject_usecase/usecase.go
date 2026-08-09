package knowledge_reproject_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/knowledge_projection_port"
	"alt/orchestrator/port/knowledge_projection_version_port"
	"alt/orchestrator/port/knowledge_reproject_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// projectorName is the checkpoint key used by the knowledge home projector.
const projectorName = "knowledge-home-projector"

// ErrNoReprojectExecutor is returned when a reproject is requested and no
// component is wired to carry it out.
//
// A run row is a request for work, not the work. Something has to advance it
// through pending → running → swappable; ADR-000421 specified a scheduled job
// in this service for that, and ADR-000944 removed the job while re-owning the
// projectors to knowledge-sovereign without naming a new home for it. Until
// one is named and wired, recording the request would leave the operator
// unable to tell "the executor was never rebuilt" from "the reproject is still
// running" — the confusion ADR-000928 exists to prevent.
var ErrNoReprojectExecutor = errors.New("no reproject executor is wired: a run would stay pending forever")

// Usecase orchestrates reproject run lifecycle.
type Usecase struct {
	// executor names the component that advances runs. Empty means none is
	// wired. Deliberately a name rather than a port: the ports below are all
	// present and working, so a nil check on any of them would report the
	// capability as available. See ErrNoReprojectExecutor.
	executor string

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

// WithExecutor declares which component advances reproject runs. Without it
// StartReproject refuses, so wiring an executor back up is a deliberate act
// rather than something a non-nil port implies.
func (u *Usecase) WithExecutor(name string) *Usecase {
	u.executor = name
	return u
}

// ExecutorWired reports whether a component is declared to advance runs. The
// composition root logs this at startup so the capability's state is visible
// before anyone presses the button.
func (u *Usecase) ExecutorWired() bool { return u.executor != "" }

// StartReproject validates the mode and creates a pending reproject run.
func (u *Usecase) StartReproject(ctx context.Context, mode, fromVersion, toVersion string, rangeStart, rangeEnd *time.Time) (*domain.ReprojectRun, error) {
	// Before the arguments: a rejection naming the mode would send the operator
	// off to fix the mode and teach them nothing about the missing executor.
	if !u.ExecutorWired() {
		return nil, ErrNoReprojectExecutor
	}

	if !domain.IsValidReprojectMode(mode) {
		return nil, fmt.Errorf("invalid reproject mode %q", mode)
	}

	if mode == domain.ReprojectModeTimeRange {
		if rangeStart == nil || rangeEnd == nil {
			return nil, fmt.Errorf("range_start and range_end are required for time_range mode")
		}
	}

	now := time.Now()
	// JSONB columns on knowledge_reproject_runs are NOT NULL with DEFAULT '{}'.
	// PostgreSQL applies DEFAULT only when the column is OMITTED from the
	// INSERT, but the driver explicitly passes every column, so a nil
	// json.RawMessage becomes NULL and trips the NOT NULL constraint.
	// Initialize to empty objects here so the INSERT carries valid JSON
	// regardless of the downstream driver wiring.
	emptyJSON := json.RawMessage([]byte(`{}`))
	run := &domain.ReprojectRun{
		ReprojectRunID:    uuid.New(),
		ProjectionName:    "knowledge_home",
		FromVersion:       fromVersion,
		ToVersion:         toVersion,
		Mode:              mode,
		Status:            domain.ReprojectStatusPending,
		RangeStart:        rangeStart,
		RangeEnd:          rangeEnd,
		CheckpointPayload: emptyJSON,
		StatsJSON:         emptyJSON,
		DiffSummaryJSON:   emptyJSON,
		CreatedAt:         now,
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

	// Defense-in-depth: dry_run mode skips event projection entirely
	// (knowledge_reproject_job.go:99-114), so the resulting "swappable" run
	// has an empty target version. Activating it would flip the read path
	// to a projection with zero rows — the canonical PM-2026-041 symptom
	// (empty link kicker on every Knowledge Home article). Reject at the
	// usecase boundary so the admin UI cannot accidentally activate a
	// preview-only projection regardless of which entry point invokes it.
	if run.Mode == domain.ReprojectModeDryRun {
		return fmt.Errorf("cannot swap dry_run reproject run %s: dry_run mode does not project events; use mode=full|user_subset|time_range", run.ReprojectRunID)
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
