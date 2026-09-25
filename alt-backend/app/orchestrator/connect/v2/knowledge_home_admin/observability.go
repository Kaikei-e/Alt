package knowledge_home_admin

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/utils/safeconv"
)

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
		ActiveVersion: safeconv.Int32(health.ActiveVersion),
		CheckpointSeq: health.CheckpointSeq,
		LastUpdated:   health.LastUpdated.Format(time.RFC3339),
		BackfillJobs:  protoJobs,
	}), nil
}

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

	return connect.NewResponse(&knowledgehomev1.GetSLOStatusResponse{
		OverallHealth:         status.OverallHealth,
		Slis:                  convertSLIStatuses(status.SLIs),
		ErrorBudgetWindowDays: safeconv.Int32(status.ErrorBudgetWindowDays),
		ActiveAlerts:          convertAlertSummaries(status.ActiveAlerts),
		ComputedAt:            status.ComputedAt.Format(time.RFC3339),
	}), nil
}

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
		Audit: convertProjectionAudit(audit),
	}), nil
}

// GetSystemMetrics returns aggregated system metrics from OTel instrumentation and service health.
func (h *Handler) GetSystemMetrics(
	ctx context.Context,
	_ *connect.Request[knowledgehomev1.GetSystemMetricsRequest],
) (*connect.Response[knowledgehomev1.GetSystemMetricsResponse], error) {
	metrics, err := h.metricsUsecase.GetSystemMetrics(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get system metrics", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get system metrics: %w", err))
	}

	return connect.NewResponse(convertSystemMetrics(metrics)), nil
}
