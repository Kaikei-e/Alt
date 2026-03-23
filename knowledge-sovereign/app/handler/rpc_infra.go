package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/driver/sovereign_db"
)

// === Events ===

func (h *SovereignHandler) ListKnowledgeEvents(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListKnowledgeEventsRequest],
) (*connect.Response[sovereignv1.ListKnowledgeEventsResponse], error) {
	msg := req.Msg
	var events []sovereign_db.KnowledgeEvent
	var err error

	if msg.UserId != "" {
		userID := parseUUID(msg.UserId)
		events, err = h.readDB.ListKnowledgeEventsSinceForUser(ctx, userID, msg.AfterSeq, int(msg.Limit))
	} else {
		events, err = h.readDB.ListKnowledgeEventsSince(ctx, msg.AfterSeq, int(msg.Limit))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListKnowledgeEvents: %w", err))
	}

	pbEvents := make([]*sovereignv1.KnowledgeEvent, len(events))
	for i, e := range events {
		pbEvents[i] = eventToProto(e)
	}
	return connect.NewResponse(&sovereignv1.ListKnowledgeEventsResponse{Events: pbEvents}), nil
}

func (h *SovereignHandler) GetLatestEventSeq(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetLatestEventSeqRequest],
) (*connect.Response[sovereignv1.GetLatestEventSeqResponse], error) {
	userID := parseUUID(req.Msg.UserId)
	seq, err := h.readDB.GetLatestKnowledgeEventSeqForUser(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetLatestEventSeq: %w", err))
	}
	return connect.NewResponse(&sovereignv1.GetLatestEventSeqResponse{EventSeq: seq}), nil
}

func (h *SovereignHandler) AppendKnowledgeEvent(
	ctx context.Context,
	req *connect.Request[sovereignv1.AppendKnowledgeEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeEventResponse], error) {
	pe := req.Msg.Event
	if pe == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("event is required"))
	}

	event := sovereign_db.KnowledgeEvent{
		EventID:       parseUUID(pe.EventId),
		ActorType:     pe.ActorType,
		ActorID:       pe.ActorId,
		EventType:     pe.EventType,
		AggregateType: pe.AggregateType,
		AggregateID:   pe.AggregateId,
		DedupeKey:     pe.DedupeKey,
		Payload:       json.RawMessage(pe.Payload),
		TenantID:      parseUUID(pe.TenantId),
		UserID:        parseUUIDPtr(pe.UserId),
		CorrelationID: parseUUIDPtr(pe.CorrelationId),
		CausationID:   parseUUIDPtr(pe.CausationId),
	}
	if pe.OccurredAt != nil {
		event.OccurredAt = pe.OccurredAt.AsTime()
	} else {
		event.OccurredAt = time.Now()
	}

	seq, err := h.readDB.AppendKnowledgeEvent(ctx, event)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendKnowledgeEvent: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendKnowledgeEventResponse{EventSeq: seq}), nil
}

// === Projection versions ===

func (h *SovereignHandler) GetActiveProjectionVersion(
	ctx context.Context,
	_ *connect.Request[sovereignv1.GetActiveProjectionVersionRequest],
) (*connect.Response[sovereignv1.GetActiveProjectionVersionResponse], error) {
	v, err := h.readDB.GetActiveProjectionVersion(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetActiveProjectionVersion: %w", err))
	}
	var pb *sovereignv1.ProjectionVersion
	if v != nil {
		pb = projectionVersionToProto(*v)
	}
	return connect.NewResponse(&sovereignv1.GetActiveProjectionVersionResponse{Version: pb}), nil
}

func (h *SovereignHandler) ListProjectionVersions(
	ctx context.Context,
	_ *connect.Request[sovereignv1.ListProjectionVersionsRequest],
) (*connect.Response[sovereignv1.ListProjectionVersionsResponse], error) {
	versions, err := h.readDB.ListProjectionVersions(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListProjectionVersions: %w", err))
	}
	pb := make([]*sovereignv1.ProjectionVersion, len(versions))
	for i, v := range versions {
		pb[i] = projectionVersionToProto(v)
	}
	return connect.NewResponse(&sovereignv1.ListProjectionVersionsResponse{Versions: pb}), nil
}

func (h *SovereignHandler) CreateProjectionVersion(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateProjectionVersionRequest],
) (*connect.Response[sovereignv1.CreateProjectionVersionResponse], error) {
	pv := req.Msg.Version
	if pv == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("version is required"))
	}
	v := sovereign_db.ProjectionVersion{
		Version:     int(pv.Version),
		Description: pv.Description,
		Status:      pv.Status,
	}
	if pv.CreatedAt != nil {
		v.CreatedAt = pv.CreatedAt.AsTime()
	} else {
		v.CreatedAt = time.Now()
	}
	if pv.ActivatedAt != nil {
		t := pv.ActivatedAt.AsTime()
		v.ActivatedAt = &t
	}
	if err := h.readDB.CreateProjectionVersion(ctx, v); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateProjectionVersion: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateProjectionVersionResponse{}), nil
}

func (h *SovereignHandler) ActivateProjectionVersion(
	ctx context.Context,
	req *connect.Request[sovereignv1.ActivateProjectionVersionRequest],
) (*connect.Response[sovereignv1.ActivateProjectionVersionResponse], error) {
	if err := h.readDB.ActivateProjectionVersion(ctx, int(req.Msg.Version)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ActivateProjectionVersion: %w", err))
	}
	return connect.NewResponse(&sovereignv1.ActivateProjectionVersionResponse{}), nil
}

// === Checkpoints ===

func (h *SovereignHandler) GetProjectionCheckpoint(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetProjectionCheckpointRequest],
) (*connect.Response[sovereignv1.GetProjectionCheckpointResponse], error) {
	seq, err := h.readDB.GetProjectionCheckpoint(ctx, req.Msg.ProjectorName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetProjectionCheckpoint: %w", err))
	}
	return connect.NewResponse(&sovereignv1.GetProjectionCheckpointResponse{LastEventSeq: seq}), nil
}

func (h *SovereignHandler) UpdateProjectionCheckpoint(
	ctx context.Context,
	req *connect.Request[sovereignv1.UpdateProjectionCheckpointRequest],
) (*connect.Response[sovereignv1.UpdateProjectionCheckpointResponse], error) {
	if err := h.readDB.UpdateProjectionCheckpoint(ctx, req.Msg.ProjectorName, req.Msg.LastEventSeq); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("UpdateProjectionCheckpoint: %w", err))
	}
	return connect.NewResponse(&sovereignv1.UpdateProjectionCheckpointResponse{}), nil
}

func (h *SovereignHandler) GetProjectionFreshness(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetProjectionFreshnessRequest],
) (*connect.Response[sovereignv1.GetProjectionFreshnessResponse], error) {
	t, err := h.readDB.GetProjectionFreshness(ctx, req.Msg.ProjectorName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetProjectionFreshness: %w", err))
	}
	resp := &sovereignv1.GetProjectionFreshnessResponse{Found: t != nil}
	if t != nil {
		resp.UpdatedAt = timestamppb.New(*t)
	}
	return connect.NewResponse(resp), nil
}

func (h *SovereignHandler) GetProjectionLag(
	ctx context.Context,
	_ *connect.Request[sovereignv1.GetProjectionLagRequest],
) (*connect.Response[sovereignv1.GetProjectionLagResponse], error) {
	lag, err := h.readDB.GetProjectionLag(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetProjectionLag: %w", err))
	}
	age, err := h.readDB.GetProjectionAge(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetProjectionAge: %w", err))
	}
	return connect.NewResponse(&sovereignv1.GetProjectionLagResponse{LagSeconds: lag, AgeSeconds: age}), nil
}

// === Reproject ===

func (h *SovereignHandler) GetReprojectRun(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetReprojectRunRequest],
) (*connect.Response[sovereignv1.GetReprojectRunResponse], error) {
	runID := parseUUID(req.Msg.RunId)
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
	run := protoToReprojectRun(req.Msg.Run)
	if err := h.readDB.CreateReprojectRun(ctx, run); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateReprojectRun: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateReprojectRunResponse{}), nil
}

func (h *SovereignHandler) UpdateReprojectRun(
	ctx context.Context,
	req *connect.Request[sovereignv1.UpdateReprojectRunRequest],
) (*connect.Response[sovereignv1.UpdateReprojectRunResponse], error) {
	run := protoToReprojectRun(req.Msg.Run)
	if err := h.readDB.UpdateReprojectRun(ctx, run); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("UpdateReprojectRun: %w", err))
	}
	return connect.NewResponse(&sovereignv1.UpdateReprojectRunResponse{}), nil
}

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
	pa := req.Msg.Audit
	if pa == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("audit is required"))
	}
	audit := sovereign_db.ProjectionAudit{
		AuditID:           parseUUID(pa.AuditId),
		ProjectionName:    pa.ProjectionName,
		ProjectionVersion: pa.ProjectionVersion,
		SampleSize:        int(pa.SampleSize),
		MismatchCount:     int(pa.MismatchCount),
		DetailsJSON:       pa.DetailsJson,
	}
	if pa.CheckedAt != nil {
		audit.CheckedAt = pa.CheckedAt.AsTime()
	}
	if err := h.readDB.CreateProjectionAudit(ctx, audit); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateProjectionAudit: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateProjectionAuditResponse{}), nil
}

// === Backfill ===

func (h *SovereignHandler) GetBackfillJob(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetBackfillJobRequest],
) (*connect.Response[sovereignv1.GetBackfillJobResponse], error) {
	jobID := parseUUID(req.Msg.JobId)
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
	j := protoToBackfillJob(req.Msg.Job)
	if err := h.readDB.CreateBackfillJob(ctx, j); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateBackfillJob: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateBackfillJobResponse{}), nil
}

func (h *SovereignHandler) UpdateBackfillJob(
	ctx context.Context,
	req *connect.Request[sovereignv1.UpdateBackfillJobRequest],
) (*connect.Response[sovereignv1.UpdateBackfillJobResponse], error) {
	j := protoToBackfillJob(req.Msg.Job)
	if err := h.readDB.UpdateBackfillJob(ctx, j); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("UpdateBackfillJob: %w", err))
	}
	return connect.NewResponse(&sovereignv1.UpdateBackfillJobResponse{}), nil
}

// === Recall Signals ===

func (h *SovereignHandler) ListRecallSignals(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListRecallSignalsRequest],
) (*connect.Response[sovereignv1.ListRecallSignalsResponse], error) {
	userID := parseUUID(req.Msg.UserId)
	signals, err := h.readDB.ListRecallSignalsByUser(ctx, userID, int(req.Msg.SinceDays))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListRecallSignals: %w", err))
	}
	pb := make([]*sovereignv1.RecallSignal, len(signals))
	for i, s := range signals {
		pb[i] = &sovereignv1.RecallSignal{
			SignalId:       s.SignalID.String(),
			UserId:         s.UserID.String(),
			ItemKey:        s.ItemKey,
			SignalType:     s.SignalType,
			SignalStrength: s.SignalStrength,
			OccurredAt:     timestamppb.New(s.OccurredAt),
			Payload:        s.Payload,
		}
	}
	return connect.NewResponse(&sovereignv1.ListRecallSignalsResponse{Signals: pb}), nil
}

func (h *SovereignHandler) AppendRecallSignal(
	ctx context.Context,
	req *connect.Request[sovereignv1.AppendRecallSignalRequest],
) (*connect.Response[sovereignv1.AppendRecallSignalResponse], error) {
	ps := req.Msg.Signal
	if ps == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("signal is required"))
	}
	s := sovereign_db.RecallSignal{
		SignalID:       parseUUID(ps.SignalId),
		UserID:         parseUUID(ps.UserId),
		ItemKey:        ps.ItemKey,
		SignalType:     ps.SignalType,
		SignalStrength: ps.SignalStrength,
		Payload:        ps.Payload,
	}
	if ps.OccurredAt != nil {
		s.OccurredAt = ps.OccurredAt.AsTime()
	} else {
		s.OccurredAt = time.Now()
	}
	if err := h.readDB.AppendRecallSignal(ctx, s); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendRecallSignal: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendRecallSignalResponse{}), nil
}

// === User events ===

func (h *SovereignHandler) AppendKnowledgeUserEvent(
	ctx context.Context,
	req *connect.Request[sovereignv1.AppendKnowledgeUserEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeUserEventResponse], error) {
	pe := req.Msg.Event
	if pe == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("event is required"))
	}
	e := sovereign_db.KnowledgeUserEvent{
		UserEventID: parseUUID(pe.UserEventId),
		UserID:      parseUUID(pe.UserId),
		TenantID:    parseUUID(pe.TenantId),
		EventType:   pe.EventType,
		ItemKey:     pe.ItemKey,
		Payload:     pe.Payload,
		DedupeKey:   pe.DedupeKey,
	}
	if pe.OccurredAt != nil {
		e.OccurredAt = pe.OccurredAt.AsTime()
	} else {
		e.OccurredAt = time.Now()
	}
	if err := h.readDB.AppendKnowledgeUserEvent(ctx, e); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendKnowledgeUserEvent: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendKnowledgeUserEventResponse{}), nil
}

// === Proto conversion helpers ===

func eventToProto(e sovereign_db.KnowledgeEvent) *sovereignv1.KnowledgeEvent {
	pb := &sovereignv1.KnowledgeEvent{
		EventId:       e.EventID.String(),
		EventSeq:      e.EventSeq,
		OccurredAt:    timestamppb.New(e.OccurredAt),
		TenantId:      e.TenantID.String(),
		ActorType:     e.ActorType,
		ActorId:       e.ActorID,
		EventType:     e.EventType,
		AggregateType: e.AggregateType,
		AggregateId:   e.AggregateID,
		DedupeKey:     e.DedupeKey,
		Payload:       e.Payload,
	}
	if e.UserID != nil {
		pb.UserId = e.UserID.String()
	}
	if e.CorrelationID != nil {
		pb.CorrelationId = e.CorrelationID.String()
	}
	if e.CausationID != nil {
		pb.CausationId = e.CausationID.String()
	}
	return pb
}

func projectionVersionToProto(v sovereign_db.ProjectionVersion) *sovereignv1.ProjectionVersion {
	pb := &sovereignv1.ProjectionVersion{
		Version:     int32(v.Version),
		Description: v.Description,
		Status:      v.Status,
		CreatedAt:   timestamppb.New(v.CreatedAt),
	}
	if v.ActivatedAt != nil {
		pb.ActivatedAt = timestamppb.New(*v.ActivatedAt)
	}
	return pb
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

func protoToReprojectRun(pb *sovereignv1.ReprojectRun) sovereign_db.ReprojectRun {
	if pb == nil {
		return sovereign_db.ReprojectRun{}
	}
	r := sovereign_db.ReprojectRun{
		ReprojectRunID: parseUUID(pb.ReprojectRunId),
		ProjectionName: pb.ProjectionName,
		FromVersion:    pb.FromVersion,
		ToVersion:      pb.ToVersion,
		InitiatedBy:    parseUUIDPtr(pb.InitiatedBy),
		Mode:           pb.Mode,
		Status:         pb.Status,
		CheckpointPayload: pb.CheckpointPayload,
		StatsJSON:      pb.StatsJson,
		DiffSummaryJSON: pb.DiffSummaryJson,
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
	return r
}

func backfillJobToProto(j sovereign_db.BackfillJob) *sovereignv1.BackfillJob {
	pb := &sovereignv1.BackfillJob{
		JobId:             j.JobID.String(),
		Status:            j.Status,
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

func protoToBackfillJob(pb *sovereignv1.BackfillJob) sovereign_db.BackfillJob {
	if pb == nil {
		return sovereign_db.BackfillJob{}
	}
	j := sovereign_db.BackfillJob{
		JobID:             parseUUID(pb.JobId),
		Status:            pb.Status,
		ProjectionVersion: int(pb.ProjectionVersion),
		TotalEvents:       int(pb.TotalEvents),
		ProcessedEvents:   int(pb.ProcessedEvents),
		ErrorMessage:      pb.ErrorMessage,
		CursorUserID:      parseUUIDPtr(pb.CursorUserId),
		CursorArticleID:   parseUUIDPtr(pb.CursorArticleId),
	}
	if pb.CursorDate != "" {
		t, _ := time.Parse("2006-01-02", pb.CursorDate)
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
	return j
}
