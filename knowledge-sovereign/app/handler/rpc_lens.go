package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func (h *SovereignHandler) ListLenses(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListLensesRequest],
) (*connect.Response[sovereignv1.ListLensesResponse], error) {
	userID, err := parseUUIDField("user_id", req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lenses, err := h.readDB.ListLenses(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListLenses: %w", err))
	}
	pb := make([]*sovereignv1.Lens, len(lenses))
	for i, l := range lenses {
		pb[i] = lensToProto(l)
	}
	return connect.NewResponse(&sovereignv1.ListLensesResponse{Lenses: pb}), nil
}

func (h *SovereignHandler) GetLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetLensRequest],
) (*connect.Response[sovereignv1.GetLensResponse], error) {
	lensID, err := parseUUIDField("lens_id", req.Msg.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lens, err := h.readDB.GetLens(ctx, lensID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetLens: %w", err))
	}
	var pb *sovereignv1.Lens
	if lens != nil {
		version, err := h.readDB.GetCurrentLensVersion(ctx, lensID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetLens version: %w", err))
		}
		lens.CurrentVersion = version
		pb = lensToProto(*lens)
	}
	return connect.NewResponse(&sovereignv1.GetLensResponse{Lens: pb}), nil
}

func (h *SovereignHandler) GetCurrentLensSelection(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetCurrentLensSelectionRequest],
) (*connect.Response[sovereignv1.GetCurrentLensSelectionResponse], error) {
	userID, err := parseUUIDField("user_id", req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	sel, err := h.readDB.GetCurrentLensSelection(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetCurrentLensSelection: %w", err))
	}
	resp := &sovereignv1.GetCurrentLensSelectionResponse{Found: sel != nil}
	if sel != nil {
		resp.Selection = &sovereignv1.CurrentLensSelection{
			UserId:        sel.UserID.String(),
			LensId:        sel.LensID.String(),
			LensVersionId: sel.LensVersionID.String(),
			SelectedAt:    timestamppb.New(sel.SelectedAt),
		}
	}
	return connect.NewResponse(resp), nil
}

func (h *SovereignHandler) ResolveLensFilter(
	ctx context.Context,
	req *connect.Request[sovereignv1.ResolveLensFilterRequest],
) (*connect.Response[sovereignv1.ResolveLensFilterResponse], error) {
	userID, err := parseUUIDField("user_id", req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lensID, err := parseUUIDPtrField("lens_id", req.Msg.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	filter, err := h.readDB.ResolveLensFilter(ctx, userID, lensID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ResolveLensFilter: %w", err))
	}
	resp := &sovereignv1.ResolveLensFilterResponse{Found: filter != nil}
	if filter != nil {
		resp.Filter = &sovereignv1.LensFilter{
			TagIds:       filter.TagNames,
			SourceIds:    filter.SourceIDs,
			TimeWindow:   filter.TimeWindow,
			QueryText:    filter.QueryText,
			IncludeRecap: filter.IncludeRecap,
			IncludePulse: filter.IncludePulse,
			SortMode:     filter.SortMode,
		}
	}
	return connect.NewResponse(resp), nil
}

func (h *SovereignHandler) CreateLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateLensRequest],
) (*connect.Response[sovereignv1.CreateLensResponse], error) {
	pl := req.Msg.Lens
	if pl == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lens is required"))
	}
	lensID, err := parseUUIDField("lens_id", pl.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	userID, err := parseUUIDField("user_id", pl.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID, err := parseUUIDField("tenant_id", pl.TenantId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if pl.CreatedAt == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("created_at is required"))
	}
	l := sovereign_db.KnowledgeLens{
		LensID:      lensID,
		UserID:      userID,
		TenantID:    tenantID,
		Name:        pl.Name,
		Description: pl.Description,
		CreatedAt:   pl.CreatedAt.AsTime(),
	}
	l.UpdatedAt = l.CreatedAt
	if err := h.readDB.CreateLens(ctx, l); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateLens: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateLensResponse{}), nil
}

func (h *SovereignHandler) CreateLensVersion(
	ctx context.Context,
	req *connect.Request[sovereignv1.CreateLensVersionRequest],
) (*connect.Response[sovereignv1.CreateLensVersionResponse], error) {
	pv := req.Msg.Version
	if pv == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("version is required"))
	}
	lensVersionID, err := parseUUIDField("lens_version_id", pv.LensVersionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lensID, err := parseUUIDField("lens_id", pv.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if pv.CreatedAt == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("created_at is required"))
	}
	v := sovereign_db.KnowledgeLensVersion{
		LensVersionID: lensVersionID,
		LensID:        lensID,
		QueryText:     pv.QueryText,
		TagIDs:        pv.TagIds,
		SourceIDs:     pv.SourceIds,
		TimeWindow:    pv.TimeWindow,
		IncludeRecap:  pv.IncludeRecap,
		IncludePulse:  pv.IncludePulse,
		SortMode:      pv.SortMode,
		CreatedAt:     pv.CreatedAt.AsTime(),
	}
	if err := h.readDB.CreateLensVersion(ctx, v); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("CreateLensVersion: %w", err))
	}
	return connect.NewResponse(&sovereignv1.CreateLensVersionResponse{}), nil
}

func (h *SovereignHandler) SelectCurrentLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.SelectCurrentLensRequest],
) (*connect.Response[sovereignv1.SelectCurrentLensResponse], error) {
	ps := req.Msg.Selection
	if ps == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("selection is required"))
	}
	userID, err := parseUUIDField("user_id", ps.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lensID, err := parseUUIDField("lens_id", ps.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lensVersionID, err := parseUUIDField("lens_version_id", ps.LensVersionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if ps.SelectedAt == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("selected_at is required"))
	}
	c := sovereign_db.KnowledgeCurrentLens{
		UserID:        userID,
		LensID:        lensID,
		LensVersionID: lensVersionID,
		SelectedAt:    ps.SelectedAt.AsTime(),
	}
	if err := h.readDB.SelectCurrentLens(ctx, c); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("SelectCurrentLens: %w", err))
	}
	return connect.NewResponse(&sovereignv1.SelectCurrentLensResponse{}), nil
}

func (h *SovereignHandler) ClearCurrentLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.ClearCurrentLensRequest],
) (*connect.Response[sovereignv1.ClearCurrentLensResponse], error) {
	userID, err := parseUUIDField("user_id", req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.ClearCurrentLens(ctx, userID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ClearCurrentLens: %w", err))
	}
	return connect.NewResponse(&sovereignv1.ClearCurrentLensResponse{}), nil
}

func (h *SovereignHandler) ArchiveLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.ArchiveLensRequest],
) (*connect.Response[sovereignv1.ArchiveLensResponse], error) {
	lensID, err := parseUUIDField("lens_id", req.Msg.LensId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.ArchiveLens(ctx, lensID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ArchiveLens: %w", err))
	}
	return connect.NewResponse(&sovereignv1.ArchiveLensResponse{}), nil
}

// --- conversion helpers ---

func lensToProto(l sovereign_db.KnowledgeLens) *sovereignv1.Lens {
	pb := &sovereignv1.Lens{
		LensId:      l.LensID.String(),
		UserId:      l.UserID.String(),
		TenantId:    l.TenantID.String(),
		Name:        l.Name,
		Description: l.Description,
		CreatedAt:   timestamppb.New(l.CreatedAt),
		UpdatedAt:   timestamppb.New(l.UpdatedAt),
	}
	if l.ArchivedAt != nil {
		pb.ArchivedAt = timestamppb.New(*l.ArchivedAt)
	}
	if l.CurrentVersion != nil {
		pb.CurrentVersion = lensVersionToProto(*l.CurrentVersion)
	}
	return pb
}

func lensVersionToProto(v sovereign_db.KnowledgeLensVersion) *sovereignv1.LensVersion {
	pb := &sovereignv1.LensVersion{
		LensVersionId: v.LensVersionID.String(),
		LensId:        v.LensID.String(),
		CreatedAt:     timestamppb.New(v.CreatedAt),
		QueryText:     v.QueryText,
		TagIds:        v.TagIDs,
		SourceIds:     v.SourceIDs,
		TimeWindow:    v.TimeWindow,
		IncludeRecap:  v.IncludeRecap,
		IncludePulse:  v.IncludePulse,
		SortMode:      v.SortMode,
	}
	if v.SupersededBy != nil {
		pb.SupersededBy = v.SupersededBy.String()
	}
	return pb
}
