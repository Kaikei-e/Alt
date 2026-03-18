// Package knowledge_home_admin provides the Connect-RPC handler for KnowledgeHomeAdminService.
package knowledge_home_admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"alt/config"
	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/gen/proto/alt/knowledge_home/v1/knowledgehomev1connect"
	"alt/usecase/knowledge_backfill_usecase"
	"alt/usecase/knowledge_projection_health_usecase"
)

// ReprojectUsecase defines the reproject operations interface.
type ReprojectUsecase interface {
	StartReproject(ctx context.Context, mode, fromVersion, toVersion string, rangeStart, rangeEnd *time.Time) (*domain.ReprojectRun, error)
	GetReprojectStatus(ctx context.Context, runID uuid.UUID) (*domain.ReprojectRun, error)
	ListReprojectRuns(ctx context.Context, statusFilter string, limit int) ([]domain.ReprojectRun, error)
	CompareReproject(ctx context.Context, runID uuid.UUID) (*domain.ReprojectDiffSummary, error)
	SwapReproject(ctx context.Context, runID uuid.UUID) error
	RollbackReproject(ctx context.Context, runID uuid.UUID) error
}

// SLOUsecase defines the SLO status operations interface.
type SLOUsecase interface {
	GetSLOStatus(ctx context.Context) (*domain.SLOStatus, error)
}

// AuditUsecase defines the projection audit operations interface.
type AuditUsecase interface {
	RunProjectionAudit(ctx context.Context, projectionName, projectionVersion string, sampleSize int) (*domain.ProjectionAudit, error)
}

// Handler implements KnowledgeHomeAdminServiceHandler.
type Handler struct {
	backfillUsecase         *knowledge_backfill_usecase.Usecase
	projectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	reprojectUsecase        ReprojectUsecase
	sloUsecase              SLOUsecase
	auditUsecase            AuditUsecase
	cfg                     *config.KnowledgeHomeConfig
	logger                  *slog.Logger
}

// Compile-time interface verification.
var _ knowledgehomev1connect.KnowledgeHomeAdminServiceHandler = (*Handler)(nil)

// NewHandler creates a new KnowledgeHomeAdminService handler.
func NewHandler(
	backfill *knowledge_backfill_usecase.Usecase,
	health *knowledge_projection_health_usecase.Usecase,
	reproject ReprojectUsecase,
	slo SLOUsecase,
	audit AuditUsecase,
	cfg *config.KnowledgeHomeConfig,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		backfillUsecase:         backfill,
		projectionHealthUsecase: health,
		reprojectUsecase:        reproject,
		sloUsecase:              slo,
		auditUsecase:            audit,
		cfg:                     cfg,
		logger:                  logger,
	}
}

// TriggerBackfill starts a new backfill job.
func (h *Handler) TriggerBackfill(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TriggerBackfillRequest],
) (*connect.Response[knowledgehomev1.TriggerBackfillResponse], error) {
	version := int(req.Msg.ProjectionVersion)
	if version <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("projection_version must be positive"))
	}

	job, err := h.backfillUsecase.StartBackfill(ctx, version)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to trigger backfill", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("trigger backfill: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.TriggerBackfillResponse{
		Job: convertBackfillJob(job),
	}), nil
}

// PauseBackfill pauses a running backfill job.
func (h *Handler) PauseBackfill(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.PauseBackfillRequest],
) (*connect.Response[knowledgehomev1.PauseBackfillResponse], error) {
	jobID, err := uuid.Parse(req.Msg.JobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid job_id: %w", err))
	}

	if err := h.backfillUsecase.PauseBackfill(ctx, jobID); err != nil {
		h.logger.ErrorContext(ctx, "failed to pause backfill", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("pause backfill: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.PauseBackfillResponse{}), nil
}

// ResumeBackfill resumes a paused backfill job.
func (h *Handler) ResumeBackfill(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.ResumeBackfillRequest],
) (*connect.Response[knowledgehomev1.ResumeBackfillResponse], error) {
	jobID, err := uuid.Parse(req.Msg.JobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid job_id: %w", err))
	}

	if err := h.backfillUsecase.ResumeBackfill(ctx, jobID); err != nil {
		h.logger.ErrorContext(ctx, "failed to resume backfill", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("resume backfill: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.ResumeBackfillResponse{}), nil
}

// GetBackfillStatus returns the status of a backfill job.
func (h *Handler) GetBackfillStatus(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetBackfillStatusRequest],
) (*connect.Response[knowledgehomev1.GetBackfillStatusResponse], error) {
	jobID, err := uuid.Parse(req.Msg.JobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid job_id: %w", err))
	}

	job, err := h.backfillUsecase.GetBackfillStatus(ctx, jobID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get backfill status", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get backfill status: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.GetBackfillStatusResponse{
		Job: convertBackfillJob(job),
	}), nil
}

// GetProjectionHealth returns projection health metrics.
func (h *Handler) GetProjectionHealth(
	ctx context.Context,
	_ *connect.Request[knowledgehomev1.GetProjectionHealthRequest],
) (*connect.Response[knowledgehomev1.GetProjectionHealthResponse], error) {
	health, err := h.projectionHealthUsecase.GetHealth(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get projection health", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get projection health: %w", err))
	}

	protoJobs := make([]*knowledgehomev1.BackfillJob, 0, len(health.BackfillJobs))
	for i := range health.BackfillJobs {
		protoJobs = append(protoJobs, convertBackfillJob(&health.BackfillJobs[i]))
	}

	return connect.NewResponse(&knowledgehomev1.GetProjectionHealthResponse{
		ActiveVersion: int32(health.ActiveVersion),
		CheckpointSeq: health.CheckpointSeq,
		LastUpdated:   health.LastUpdated.Format(time.RFC3339),
		BackfillJobs:  protoJobs,
	}), nil
}

// GetFeatureFlags returns the current feature flag configuration.
func (h *Handler) GetFeatureFlags(
	_ context.Context,
	_ *connect.Request[knowledgehomev1.GetFeatureFlagsRequest],
) (*connect.Response[knowledgehomev1.GetFeatureFlagsResponse], error) {
	return connect.NewResponse(&knowledgehomev1.GetFeatureFlagsResponse{
		EnableHomePage:      h.cfg.EnableHomePage,
		EnableTracking:      h.cfg.EnableTracking,
		EnableProjectionV2:  h.cfg.EnableProjectionV2,
		RolloutPercentage:   int32(h.cfg.RolloutPercentage),
		EnableRecallRail:    h.cfg.EnableRecallRail,
		EnableLens:          h.cfg.EnableLens,
		EnableStreamUpdates: h.cfg.EnableStreamUpdates,
		EnableSupersedeUx:   h.cfg.EnableSupersedeUX,
	}), nil
}

// --- Phase 5: Reproject RPCs ---

// StartReproject initiates a new projection re-build run.
func (h *Handler) StartReproject(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.StartReprojectRequest],
) (*connect.Response[knowledgehomev1.StartReprojectResponse], error) {
	if req.Msg.Mode == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("mode is required"))
	}
	if req.Msg.FromVersion == "" || req.Msg.ToVersion == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("from_version and to_version are required"))
	}

	var rangeStart, rangeEnd *time.Time
	if req.Msg.RangeStart != nil && *req.Msg.RangeStart != "" {
		t, err := time.Parse(time.RFC3339, *req.Msg.RangeStart)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid range_start: %w", err))
		}
		rangeStart = &t
	}
	if req.Msg.RangeEnd != nil && *req.Msg.RangeEnd != "" {
		t, err := time.Parse(time.RFC3339, *req.Msg.RangeEnd)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid range_end: %w", err))
		}
		rangeEnd = &t
	}

	run, err := h.reprojectUsecase.StartReproject(ctx, req.Msg.Mode, req.Msg.FromVersion, req.Msg.ToVersion, rangeStart, rangeEnd)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to start reproject", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("start reproject: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.StartReprojectResponse{
		Run: convertReprojectRun(run),
	}), nil
}

// GetReprojectStatus returns the status of a reproject run.
func (h *Handler) GetReprojectStatus(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetReprojectStatusRequest],
) (*connect.Response[knowledgehomev1.GetReprojectStatusResponse], error) {
	runID, err := uuid.Parse(req.Msg.ReprojectRunId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid reproject_run_id: %w", err))
	}

	run, err := h.reprojectUsecase.GetReprojectStatus(ctx, runID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get reproject status", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get reproject status: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.GetReprojectStatusResponse{
		Run: convertReprojectRun(run),
	}), nil
}

// ListReprojectRuns returns all reproject runs.
func (h *Handler) ListReprojectRuns(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.ListReprojectRunsRequest],
) (*connect.Response[knowledgehomev1.ListReprojectRunsResponse], error) {
	var statusFilter string
	if req.Msg.StatusFilter != nil {
		statusFilter = *req.Msg.StatusFilter
	}
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}

	runs, err := h.reprojectUsecase.ListReprojectRuns(ctx, statusFilter, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to list reproject runs", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list reproject runs: %w", err))
	}

	protoRuns := make([]*knowledgehomev1.ReprojectRun, 0, len(runs))
	for i := range runs {
		protoRuns = append(protoRuns, convertReprojectRun(&runs[i]))
	}

	return connect.NewResponse(&knowledgehomev1.ListReprojectRunsResponse{
		Runs: protoRuns,
	}), nil
}

// CompareReproject compares two projection versions from a reproject run.
func (h *Handler) CompareReproject(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.CompareReprojectRequest],
) (*connect.Response[knowledgehomev1.CompareReprojectResponse], error) {
	runID, err := uuid.Parse(req.Msg.ReprojectRunId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid reproject_run_id: %w", err))
	}

	diff, err := h.reprojectUsecase.CompareReproject(ctx, runID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to compare reproject", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("compare reproject: %w", err))
	}

	fromWhyJSON, _ := json.Marshal(diff.FromWhyDistribution)
	toWhyJSON, _ := json.Marshal(diff.ToWhyDistribution)

	return connect.NewResponse(&knowledgehomev1.CompareReprojectResponse{
		Diff: &knowledgehomev1.ReprojectDiffSummary{
			FromItemCount:       diff.FromItemCount,
			ToItemCount:         diff.ToItemCount,
			FromEmptyCount:      diff.FromEmptyCount,
			ToEmptyCount:        diff.ToEmptyCount,
			FromAvgScore:        diff.FromAvgScore,
			ToAvgScore:          diff.ToAvgScore,
			FromWhyDistribution: string(fromWhyJSON),
			ToWhyDistribution:   string(toWhyJSON),
		},
	}), nil
}

// SwapReproject swaps the active projection version.
func (h *Handler) SwapReproject(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.SwapReprojectRequest],
) (*connect.Response[knowledgehomev1.SwapReprojectResponse], error) {
	runID, err := uuid.Parse(req.Msg.ReprojectRunId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid reproject_run_id: %w", err))
	}

	if err := h.reprojectUsecase.SwapReproject(ctx, runID); err != nil {
		h.logger.ErrorContext(ctx, "failed to swap reproject", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("swap reproject: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.SwapReprojectResponse{}), nil
}

// RollbackReproject rolls back to the previous projection version.
func (h *Handler) RollbackReproject(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.RollbackReprojectRequest],
) (*connect.Response[knowledgehomev1.RollbackReprojectResponse], error) {
	runID, err := uuid.Parse(req.Msg.ReprojectRunId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid reproject_run_id: %w", err))
	}

	if err := h.reprojectUsecase.RollbackReproject(ctx, runID); err != nil {
		h.logger.ErrorContext(ctx, "failed to rollback reproject", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("rollback reproject: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.RollbackReprojectResponse{}), nil
}

// --- Phase 5: SLO RPCs ---

// GetSLOStatus returns current SLO status and error budget.
func (h *Handler) GetSLOStatus(
	ctx context.Context,
	_ *connect.Request[knowledgehomev1.GetSLOStatusRequest],
) (*connect.Response[knowledgehomev1.GetSLOStatusResponse], error) {
	status, err := h.sloUsecase.GetSLOStatus(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get SLO status", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get SLO status: %w", err))
	}

	protoSLIs := make([]*knowledgehomev1.SLIStatus, 0, len(status.SLIs))
	for _, sli := range status.SLIs {
		protoSLIs = append(protoSLIs, &knowledgehomev1.SLIStatus{
			Name:                    sli.Name,
			CurrentValue:            sli.CurrentValue,
			TargetValue:             sli.TargetValue,
			Unit:                    sli.Unit,
			Status:                  sli.Status,
			ErrorBudgetConsumedPct:  sli.ErrorBudgetConsumedPct,
		})
	}

	protoAlerts := make([]*knowledgehomev1.AlertSummary, 0, len(status.ActiveAlerts))
	for _, alert := range status.ActiveAlerts {
		protoAlerts = append(protoAlerts, &knowledgehomev1.AlertSummary{
			AlertName:   alert.AlertName,
			Severity:    alert.Severity,
			Status:      alert.Status,
			FiredAt:     alert.FiredAt.Format(time.RFC3339),
			Description: alert.Description,
		})
	}

	return connect.NewResponse(&knowledgehomev1.GetSLOStatusResponse{
		OverallHealth:         status.OverallHealth,
		Slis:                  protoSLIs,
		ErrorBudgetWindowDays: int32(status.ErrorBudgetWindowDays),
		ActiveAlerts:          protoAlerts,
		ComputedAt:            status.ComputedAt.Format(time.RFC3339),
	}), nil
}

// --- Phase 5: Audit RPCs ---

// RunProjectionAudit samples items and verifies projection correctness.
func (h *Handler) RunProjectionAudit(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.RunProjectionAuditRequest],
) (*connect.Response[knowledgehomev1.RunProjectionAuditResponse], error) {
	if req.Msg.ProjectionName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("projection_name is required"))
	}
	if req.Msg.ProjectionVersion == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("projection_version is required"))
	}
	sampleSize := int(req.Msg.SampleSize)
	if sampleSize <= 0 {
		sampleSize = 100
	}

	audit, err := h.auditUsecase.RunProjectionAudit(ctx, req.Msg.ProjectionName, req.Msg.ProjectionVersion, sampleSize)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to run projection audit", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("run projection audit: %w", err))
	}

	return connect.NewResponse(&knowledgehomev1.RunProjectionAuditResponse{
		Audit: &knowledgehomev1.ProjectionAudit{
			AuditId:           audit.AuditID.String(),
			ProjectionName:    audit.ProjectionName,
			ProjectionVersion: audit.ProjectionVersion,
			CheckedAt:         audit.CheckedAt.Format(time.RFC3339),
			SampleSize:        int32(audit.SampleSize),
			MismatchCount:     int32(audit.MismatchCount),
			DetailsJson:       string(audit.DetailsJSON),
		},
	}), nil
}

// --- Proto conversion helpers ---

// convertBackfillJob converts a domain backfill job to proto.
func convertBackfillJob(job *domain.KnowledgeBackfillJob) *knowledgehomev1.BackfillJob {
	if job == nil {
		return nil
	}
	proto := &knowledgehomev1.BackfillJob{
		JobId:             job.JobID.String(),
		Status:            job.Status,
		ProjectionVersion: int32(job.ProjectionVersion),
		TotalEvents:       int32(job.TotalEvents),
		ProcessedEvents:   int32(job.ProcessedEvents),
		ErrorMessage:      job.ErrorMessage,
		CreatedAt:         job.CreatedAt.Format(time.RFC3339),
	}
	if job.StartedAt != nil {
		proto.StartedAt = job.StartedAt.Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		proto.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}
	return proto
}

// convertReprojectRun converts a domain reproject run to proto.
func convertReprojectRun(run *domain.ReprojectRun) *knowledgehomev1.ReprojectRun {
	if run == nil {
		return nil
	}
	proto := &knowledgehomev1.ReprojectRun{
		ReprojectRunId: run.ReprojectRunID.String(),
		ProjectionName: run.ProjectionName,
		FromVersion:    run.FromVersion,
		ToVersion:      run.ToVersion,
		Mode:           run.Mode,
		Status:         run.Status,
		StatsJson:      string(run.StatsJSON),
		DiffSummaryJson: string(run.DiffSummaryJSON),
		CreatedAt:      run.CreatedAt.Format(time.RFC3339),
	}
	if run.InitiatedBy != nil {
		proto.InitiatedBy = run.InitiatedBy.String()
	}
	if run.RangeStart != nil {
		proto.RangeStart = run.RangeStart.Format(time.RFC3339)
	}
	if run.RangeEnd != nil {
		proto.RangeEnd = run.RangeEnd.Format(time.RFC3339)
	}
	if run.StartedAt != nil {
		proto.StartedAt = run.StartedAt.Format(time.RFC3339)
	}
	if run.FinishedAt != nil {
		proto.FinishedAt = run.FinishedAt.Format(time.RFC3339)
	}
	return proto
}
