package knowledge_home_admin

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/orchestrator/usecase/knowledge_backfill_usecase"
)

// EmitArticleUrlBackfill appends ArticleUrlBackfilled corrective events
// for every article whose Knowledge Home projection currently shows an
// empty URL but whose alt-db `articles.url` is non-empty and http(s)-
// scheme. Idempotent across retries via the
// `article-url-backfill:<article_id>` dedupe namespace.
//
// See ADR-000869, and the operator runbook in
// docs/runbooks/knowledge-home-reproject-operations.md
// §Post-tag-fix backfill.
func (h *Handler) EmitArticleUrlBackfill(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.EmitArticleUrlBackfillRequest],
) (*connect.Response[knowledgehomev1.EmitArticleUrlBackfillResponse], error) {
	max := int(req.Msg.MaxArticles)
	if max < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("max_articles must be non-negative"))
	}

	res, err := h.urlBackfillUsecase.Emit(ctx, max, req.Msg.DryRun)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to emit article url backfill",
			"error", err,
			"max_articles", max,
			"dry_run", req.Msg.DryRun,
		)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("emit article url backfill: %w", err))
	}

	h.logger.InfoContext(ctx, "emitted article url backfill",
		"articles_scanned", res.ArticlesScanned,
		"events_appended", res.EventsAppended,
		"skipped_blocked_scheme", res.SkippedBlockedScheme,
		"skipped_duplicate", res.SkippedDuplicate,
		"more_remaining", res.MoreRemaining,
		"dry_run", req.Msg.DryRun,
	)

	return connect.NewResponse(convertEmitArticleUrlBackfillResponse(res)), nil
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
		if errors.Is(err, knowledge_backfill_usecase.ErrNoBackfillExecutor) {
			h.logger.ErrorContext(ctx, "backfill requested but no executor is wired", "error", err)
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
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
