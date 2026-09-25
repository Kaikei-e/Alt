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

// === Projection Versions ===

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
	v, err := validateAndBuildProjectionVersion(req.Msg.Version, time.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
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

// === Checkpoints, Freshness & Lag ===

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

// === Pure Validators and Proto Conversion Mappers ===

func validateAndBuildProjectionVersion(pv *sovereignv1.ProjectionVersion, refTime time.Time) (sovereign_db.ProjectionVersion, error) {
	if pv == nil {
		return sovereign_db.ProjectionVersion{}, fmt.Errorf("version is required")
	}
	v := sovereign_db.ProjectionVersion{
		Version:     int(pv.Version),
		Description: pv.Description,
		Status:      pv.Status,
	}
	if pv.CreatedAt != nil {
		v.CreatedAt = pv.CreatedAt.AsTime()
	} else {
		v.CreatedAt = refTime
	}
	if pv.ActivatedAt != nil {
		t := pv.ActivatedAt.AsTime()
		v.ActivatedAt = &t
	}
	return v, nil
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
