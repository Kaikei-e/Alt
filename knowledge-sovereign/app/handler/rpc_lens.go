package handler

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/driver/sovereign_db"
)

func (h *SovereignHandler) ListLenses(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListLensesRequest],
) (*connect.Response[sovereignv1.ListLensesResponse], error) {
	userID := parseUUID(req.Msg.UserId)
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
	lensID := parseUUID(req.Msg.LensId)
	lens, err := h.readDB.GetLens(ctx, lensID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetLens: %w", err))
	}
	var pb *sovereignv1.Lens
	if lens != nil {
		pb = lensToProto(*lens)
	}
	return connect.NewResponse(&sovereignv1.GetLensResponse{Lens: pb}), nil
}

func (h *SovereignHandler) GetCurrentLensSelection(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetCurrentLensSelectionRequest],
) (*connect.Response[sovereignv1.GetCurrentLensSelectionResponse], error) {
	userID := parseUUID(req.Msg.UserId)
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
	userID := parseUUID(req.Msg.UserId)
	filter, err := h.readDB.ResolveLensFilter(ctx, userID, parseUUIDPtr(req.Msg.LensId))
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
	l := sovereign_db.KnowledgeLens{
		LensID:      parseUUID(pl.LensId),
		UserID:      parseUUID(pl.UserId),
		TenantID:    parseUUID(pl.TenantId),
		Name:        pl.Name,
		Description: pl.Description,
	}
	if pl.CreatedAt != nil {
		l.CreatedAt = pl.CreatedAt.AsTime()
	} else {
		l.CreatedAt = time.Now()
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
	v := sovereign_db.KnowledgeLensVersion{
		LensVersionID: parseUUID(pv.LensVersionId),
		LensID:        parseUUID(pv.LensId),
		QueryText:     pv.QueryText,
		TagIDs:        pv.TagIds,
		SourceIDs:     pv.SourceIds,
		TimeWindow:    pv.TimeWindow,
		IncludeRecap:  pv.IncludeRecap,
		IncludePulse:  pv.IncludePulse,
		SortMode:      pv.SortMode,
	}
	if pv.CreatedAt != nil {
		v.CreatedAt = pv.CreatedAt.AsTime()
	} else {
		v.CreatedAt = time.Now()
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
	c := sovereign_db.KnowledgeCurrentLens{
		UserID:        parseUUID(ps.UserId),
		LensID:        parseUUID(ps.LensId),
		LensVersionID: parseUUID(ps.LensVersionId),
	}
	if ps.SelectedAt != nil {
		c.SelectedAt = ps.SelectedAt.AsTime()
	} else {
		c.SelectedAt = time.Now()
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
	userID := parseUUID(req.Msg.UserId)
	if err := h.readDB.ClearCurrentLens(ctx, userID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ClearCurrentLens: %w", err))
	}
	return connect.NewResponse(&sovereignv1.ClearCurrentLensResponse{}), nil
}

func (h *SovereignHandler) ArchiveLens(
	ctx context.Context,
	req *connect.Request[sovereignv1.ArchiveLensRequest],
) (*connect.Response[sovereignv1.ArchiveLensResponse], error) {
	lensID := parseUUID(req.Msg.LensId)
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
