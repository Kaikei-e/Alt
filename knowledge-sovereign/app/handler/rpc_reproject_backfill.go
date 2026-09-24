package handler

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// === Reproject Runs ===

func (h *SovereignHandler) GetReprojectRun(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetReprojectRunRequest],
) (*connect.Response[sovereignv1.GetReprojectRunResponse], error) {
	runID, err := parseUUIDField("run_id", req.Msg.RunId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	run, err := h.readDB.GetReprojectRun(ctx, runID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetReprojectRun: %w", err))
	}
	var pb *sovereignv1.ReprojectRun
	if run != nil {
		pb = reprojectRunToProto(*run)
	}
	return connect.NewResponse(&sovereignv1.GetReprojectRunResponse{Run: pb}), nil
}

func (h *SovereignHandler) ListReprojectRuns(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListReprojectRunsRequest],
) (*connect.Response[sovereignv1.ListReprojectRunsResponse], error) {
	runs, err := h.readDB.ListReprojectRuns(ctx, req.Msg.StatusFilter, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListReprojectRuns: %w", err))
	}
	pb := make([]*sovereignv1.ReprojectRun, len(runs))
	for i, r := range runs {
		pb[i] = reprojectRunToProto(r)
	}
	return connect.NewResponse(&sovereignv1.ListReprojectRunsResponse{Runs: pb}), nil
}

func (h *SovereignHandler) CreateReprojectRun(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateReprojectRunRequest],
) (*connect.Response[sovereignv1.CreateReprojectRunResponse], error) {
	run, err := protoToReprojectRun(req.Msg.Run)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.CreateReprojectRun(ctx, run); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateReprojectRun: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateReprojectRunResponse{}), nil
}

func (h *SovereignHandler) UpdateReprojectRun(
	ctx context.Context,
	req *connect.Request[sovereignv1.UpdateReprojectRunRequest],
) (*connect.Response[sovereignv1.UpdateReprojectRunResponse], error) {
	run, err := protoToReprojectRun(req.Msg.Run)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.UpdateReprojectRun(ctx, run); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("UpdateReprojectRun: %w", err))
	}
	return connect.NewResponse(&sovereignv1.UpdateReprojectRunResponse{}), nil
}

// === Projections Diff & Audits ===

func (h *SovereignHandler) CompareProjections(
	ctx context.Context,
	req *connect.Request[sovereignv1.CompareProjectionsRequest],
) (*connect.Response[sovereignv1.CompareProjectionsResponse], error) {
	summary, err := h.readDB.CompareProjections(ctx, req.Msg.FromVersion, req.Msg.ToVersion)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CompareProjections: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CompareProjectionsResponse{
		Summary: &sovereignv1.ReprojectDiffSummary{
			FromCount:        int32(summary.FromCount),
			ToCount:          int32(summary.ToCount),
			FromAvgScore:     summary.FromAvgScore,
			ToAvgScore:       summary.ToAvgScore,
			FromEmptySummary: int32(summary.FromEmptySummary),
			ToEmptySummary:   int32(summary.ToEmptySummary),
		},
	}), nil
}

func (h *SovereignHandler) ListProjectionAudits(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListProjectionAuditsRequest],
) (*connect.Response[sovereignv1.ListProjectionAuditsResponse], error) {
	audits, err := h.readDB.ListProjectionAudits(ctx, req.Msg.ProjectionName, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListProjectionAudits: %w", err))
	}
	pb := make([]*sovereignv1.ProjectionAudit, len(audits))
	for i, a := range audits {
		pb[i] = &sovereignv1.ProjectionAudit{
			AuditId:           a.AuditID.String(),
			ProjectionName:    a.ProjectionName,
			ProjectionVersion: a.ProjectionVersion,
			CheckedAt:         timestamppb.New(a.CheckedAt),
			SampleSize:        int32(a.SampleSize),
			MismatchCount:     int32(a.MismatchCount),
			DetailsJson:       a.DetailsJSON,
		}
	}
	return connect.NewResponse(&sovereignv1.ListProjectionAuditsResponse{Audits: pb}), nil
}

func (h *SovereignHandler) CreateProjectionAudit(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateProjectionAuditRequest],
) (*connect.Response[sovereignv1.CreateProjectionAuditResponse], error) {
	audit, err := validateAndBuildProjectionAudit(req.Msg.Audit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.CreateProjectionAudit(ctx, audit); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateProjectionAudit: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateProjectionAuditResponse{}), nil
}

// === Backfill Jobs ===

func (h *SovereignHandler) GetBackfillJob(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetBackfillJobRequest],
) (*connect.Response[sovereignv1.GetBackfillJobResponse], error) {
	jobID, err := parseUUIDField("job_id", req.Msg.JobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	job, err := h.readDB.GetBackfillJob(ctx, jobID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetBackfillJob: %w", err))
	}
	var pb *sovereignv1.BackfillJob
	if job != nil {
		pb = backfillJobToProto(*job)
	}
	return connect.NewResponse(&sovereignv1.GetBackfillJobResponse{Job: pb}), nil
}

func (h *SovereignHandler) ListBackfillJobs(
	ctx context.Context,
	_ *connect.Request[sovereignv1.ListBackfillJobsRequest],
) (*connect.Response[sovereignv1.ListBackfillJobsResponse], error) {
	jobs, err := h.readDB.ListBackfillJobs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListBackfillJobs: %w", err))
	}
	pb := make([]*sovereignv1.BackfillJob, len(jobs))
	for i, j := range jobs {
		pb[i] = backfillJobToProto(j)
	}
	return connect.NewResponse(&sovereignv1.ListBackfillJobsResponse{Jobs: pb}), nil
}

func (h *SovereignHandler) CreateBackfillJob(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateBackfillJobRequest],
) (*connect.Response[sovereignv1.CreateBackfillJobResponse], error) {
	j, err := protoToBackfillJob(req.Msg.Job)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.CreateBackfillJob(ctx, j); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateBackfillJob: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateBackfillJobResponse{}), nil
}

func (h *SovereignHandler) UpdateBackfillJob(
	ctx context.Context,
	req *connect.Request[sovereignv1.UpdateBackfillJobRequest],
) (*connect.Response[sovereignv1.UpdateBackfillJobResponse], error) {
	j, err := protoToBackfillJob(req.Msg.Job)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.UpdateBackfillJob(ctx, j); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("UpdateBackfillJob: %w", err))
	}
	return connect.NewResponse(&sovereignv1.UpdateBackfillJobResponse{}), nil
}

// === Pure Validators and Proto Conversion Mappers ===

func validateAndBuildProjectionAudit(pa *sovereignv1.ProjectionAudit) (sovereign_db.ProjectionAudit, error) {
	if pa == nil {
		return sovereign_db.ProjectionAudit{}, fmt.Errorf("audit is required")
	}
	auditID, err := parseUUIDField("audit_id", pa.AuditId)
	if err != nil {
		return sovereign_db.ProjectionAudit{}, err
	}
	audit := sovereign_db.ProjectionAudit{
		AuditID:           auditID,
		ProjectionName:    pa.ProjectionName,
		ProjectionVersion: pa.ProjectionVersion,
		SampleSize:        int(pa.SampleSize),
		MismatchCount:     int(pa.MismatchCount),
		DetailsJSON:       pa.DetailsJson,
	}
	if pa.CheckedAt != nil {
		audit.CheckedAt = pa.CheckedAt.AsTime()
	}
	return audit, nil
}

func reprojectRunToProto(r sovereign_db.ReprojectRun) *sovereignv1.ReprojectRun {
	pb := &sovereignv1.ReprojectRun{
		ReprojectRunId:    r.ReprojectRunID.String(),
		ProjectionName:    r.ProjectionName,
		FromVersion:       r.FromVersion,
		ToVersion:         r.ToVersion,
		Mode:              r.Mode,
		Status:            r.Status,
		CheckpointPayload: r.CheckpointPayload,
		StatsJson:         r.StatsJSON,
		DiffSummaryJson:   r.DiffSummaryJSON,
		CreatedAt:         timestamppb.New(r.CreatedAt),
	}
	if r.InitiatedBy != nil {
		pb.InitiatedBy = r.InitiatedBy.String()
	}
	if r.RangeStart != nil {
		pb.RangeStart = timestamppb.New(*r.RangeStart)
	}
	if r.RangeEnd != nil {
		pb.RangeEnd = timestamppb.New(*r.RangeEnd)
	}
	if r.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*r.StartedAt)
	}
	if r.FinishedAt != nil {
		pb.FinishedAt = timestamppb.New(*r.FinishedAt)
	}
	return pb
}

func protoToReprojectRun(pb *sovereignv1.ReprojectRun) (sovereign_db.ReprojectRun, error) {
	if pb == nil {
		return sovereign_db.ReprojectRun{}, nil
	}
	reprojectRunID, err := parseUUIDField("reproject_run_id", pb.ReprojectRunId)
	if err != nil {
		return sovereign_db.ReprojectRun{}, err
	}
	initiatedBy, err := parseUUIDPtrField("initiated_by", pb.InitiatedBy)
	if err != nil {
		return sovereign_db.ReprojectRun{}, err
	}
	r := sovereign_db.ReprojectRun{
		ReprojectRunID:    reprojectRunID,
		ProjectionName:    pb.ProjectionName,
		FromVersion:       pb.FromVersion,
		ToVersion:         pb.ToVersion,
		InitiatedBy:       initiatedBy,
		Mode:              pb.Mode,
		Status:            pb.Status,
		CheckpointPayload: pb.CheckpointPayload,
		StatsJSON:         pb.StatsJson,
		DiffSummaryJSON:   pb.DiffSummaryJson,
	}
	if pb.CreatedAt != nil {
		r.CreatedAt = pb.CreatedAt.AsTime()
	}
	if pb.RangeStart != nil {
		t := pb.RangeStart.AsTime()
		r.RangeStart = &t
	}
	if pb.RangeEnd != nil {
		t := pb.RangeEnd.AsTime()
		r.RangeEnd = &t
	}
	if pb.StartedAt != nil {
		t := pb.StartedAt.AsTime()
		r.StartedAt = &t
	}
	if pb.FinishedAt != nil {
		t := pb.FinishedAt.AsTime()
		r.FinishedAt = &t
	}
	return r, nil
}

func backfillJobToProto(j sovereign_db.BackfillJob) *sovereignv1.BackfillJob {
	pb := &sovereignv1.BackfillJob{
		JobId:             j.JobID.String(),
		Status:            j.Status,
		Kind:              j.Kind,
		ProjectionVersion: int32(j.ProjectionVersion),
		TotalEvents:       int32(j.TotalEvents),
		ProcessedEvents:   int32(j.ProcessedEvents),
		ErrorMessage:      j.ErrorMessage,
		CreatedAt:         timestamppb.New(j.CreatedAt),
		UpdatedAt:         timestamppb.New(j.UpdatedAt),
	}
	if j.CursorUserID != nil {
		pb.CursorUserId = j.CursorUserID.String()
	}
	if j.CursorDate != nil {
		pb.CursorDate = j.CursorDate.Format("2006-01-02")
	}
	if j.CursorArticleID != nil {
		pb.CursorArticleId = j.CursorArticleID.String()
	}
	if j.StartedAt != nil {
		pb.StartedAt = timestamppb.New(*j.StartedAt)
	}
	if j.CompletedAt != nil {
		pb.CompletedAt = timestamppb.New(*j.CompletedAt)
	}
	return pb
}

func protoToBackfillJob(pb *sovereignv1.BackfillJob) (sovereign_db.BackfillJob, error) {
	if pb == nil {
		return sovereign_db.BackfillJob{}, nil
	}
	jobID, err := parseUUIDField("job_id", pb.JobId)
	if err != nil {
		return sovereign_db.BackfillJob{}, err
	}
	cursorUserID, err := parseUUIDPtrField("cursor_user_id", pb.CursorUserId)
	if err != nil {
		return sovereign_db.BackfillJob{}, err
	}
	cursorArticleID, err := parseUUIDPtrField("cursor_article_id", pb.CursorArticleId)
	if err != nil {
		return sovereign_db.BackfillJob{}, err
	}
	j := sovereign_db.BackfillJob{
		JobID:             jobID,
		Status:            pb.Status,
		Kind:              pb.Kind,
		ProjectionVersion: int(pb.ProjectionVersion),
		TotalEvents:       int(pb.TotalEvents),
		ProcessedEvents:   int(pb.ProcessedEvents),
		ErrorMessage:      pb.ErrorMessage,
		CursorUserID:      cursorUserID,
		CursorArticleID:   cursorArticleID,
	}
	if pb.CursorDate != "" {
		t, err := time.Parse("2006-01-02", pb.CursorDate)
		if err != nil {
			return sovereign_db.BackfillJob{}, fmt.Errorf("invalid cursor_date %q: %w", pb.CursorDate, err)
		}
		j.CursorDate = &t
	}
	if pb.CreatedAt != nil {
		j.CreatedAt = pb.CreatedAt.AsTime()
	}
	if pb.StartedAt != nil {
		t := pb.StartedAt.AsTime()
		j.StartedAt = &t
	}
	if pb.CompletedAt != nil {
		t := pb.CompletedAt.AsTime()
		j.CompletedAt = &t
	}
	if pb.UpdatedAt != nil {
		j.UpdatedAt = pb.UpdatedAt.AsTime()
	}
	return j, nil
}
