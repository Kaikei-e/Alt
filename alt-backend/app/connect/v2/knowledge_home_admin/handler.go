// Package knowledge_home_admin provides the Connect-RPC handler for KnowledgeHomeAdminService.
package knowledge_home_admin

import (
	"context"
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

// Handler implements KnowledgeHomeAdminServiceHandler.
type Handler struct {
	backfillUsecase        *knowledge_backfill_usecase.Usecase
	projectionHealthUsecase *knowledge_projection_health_usecase.Usecase
	cfg                    *config.KnowledgeHomeConfig
	logger                 *slog.Logger
}

// Compile-time interface verification.
var _ knowledgehomev1connect.KnowledgeHomeAdminServiceHandler = (*Handler)(nil)

// NewHandler creates a new KnowledgeHomeAdminService handler.
func NewHandler(
	backfill *knowledge_backfill_usecase.Usecase,
	health *knowledge_projection_health_usecase.Usecase,
	cfg *config.KnowledgeHomeConfig,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		backfillUsecase:         backfill,
		projectionHealthUsecase: health,
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
