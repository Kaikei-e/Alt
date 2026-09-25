package knowledge_home_admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/knowledge_reproject_usecase"
)

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
		// A missing executor is a fact about the deployment, not a fault in
		// this request. Internal would read as transient and invite a retry
		// that cannot succeed; FailedPrecondition tells the operator the
		// system has to change first.
		if errors.Is(err, knowledge_reproject_usecase.ErrNoReprojectExecutor) {
			h.logger.ErrorContext(ctx, "reproject requested but no executor is wired", "error", err)
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
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

	return connect.NewResponse(&knowledgehomev1.CompareReprojectResponse{
		Diff: convertReprojectDiff(diff),
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
