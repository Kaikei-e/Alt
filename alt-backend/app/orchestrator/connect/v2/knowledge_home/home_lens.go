package knowledge_home

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/orchestrator/usecase/create_lens_usecase"
	"alt/orchestrator/usecase/update_lens_usecase"

	"github.com/google/uuid"
)

// CreateLens creates a new saved viewpoint.
func (h *Handler) CreateLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.CreateLensRequest],
) (*connect.Response[knowledgehomev1.CreateLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	if req.Msg.GetName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("name is required"))
	}

	input := create_lens_usecase.CreateLensInput{
		UserID:      user.UserID,
		TenantID:    user.TenantID,
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
	}

	if req.Msg.GetVersion() != nil {
		input.QueryText = req.Msg.GetVersion().GetQueryText()
		input.TagIDs = req.Msg.GetVersion().GetTagIds()
		input.SourceIDs = req.Msg.GetVersion().GetSourceIds()
		input.TimeWindow = req.Msg.GetVersion().GetTimeWindow()
		input.IncludeRecap = req.Msg.GetVersion().GetIncludeRecap()
		input.IncludePulse = req.Msg.GetVersion().GetIncludePulse()
		input.SortMode = req.Msg.GetVersion().GetSortMode()
	}

	result, err := h.createLensUsecase.Execute(ctx, input)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "CreateLens")
	}

	return connect.NewResponse(&knowledgehomev1.CreateLensResponse{
		Lens: convertLensToProto(result.Lens, &result.Version),
	}), nil
}

// UpdateLens creates a new version of an existing lens.
func (h *Handler) UpdateLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.UpdateLensRequest],
) (*connect.Response[knowledgehomev1.UpdateLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID, err := parseUUID(req.Msg.GetLensId(), "lens_id")
	if err != nil {
		return nil, err
	}

	input := update_lens_usecase.UpdateLensInput{
		LensID: lensID,
		UserID: user.UserID,
	}

	if req.Msg.GetVersion() != nil {
		input.QueryText = req.Msg.GetVersion().GetQueryText()
		input.TagIDs = req.Msg.GetVersion().GetTagIds()
		input.SourceIDs = req.Msg.GetVersion().GetSourceIds()
		input.TimeWindow = req.Msg.GetVersion().GetTimeWindow()
		input.IncludeRecap = req.Msg.GetVersion().GetIncludeRecap()
		input.IncludePulse = req.Msg.GetVersion().GetIncludePulse()
		input.SortMode = req.Msg.GetVersion().GetSortMode()
	}

	version, err := h.updateLensUsecase.Execute(ctx, input)
	if err != nil {
		if errors.Is(err, domain.ErrLensNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, domain.ErrLensNotFound)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "UpdateLens")
	}

	return connect.NewResponse(&knowledgehomev1.UpdateLensResponse{
		Lens: &knowledgehomev1.Lens{
			LensId:         req.Msg.GetLensId(),
			CurrentVersion: convertLensVersionToProto(*version),
		},
	}), nil
}

// DeleteLens archives a lens (soft delete).
func (h *Handler) DeleteLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.DeleteLensRequest],
) (*connect.Response[knowledgehomev1.DeleteLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID, err := parseUUID(req.Msg.GetLensId(), "lens_id")
	if err != nil {
		return nil, err
	}

	if err := h.archiveLensUsecase.Execute(ctx, user.UserID, lensID); err != nil {
		if errors.Is(err, domain.ErrLensNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, domain.ErrLensNotFound)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "DeleteLens")
	}

	return connect.NewResponse(&knowledgehomev1.DeleteLensResponse{}), nil
}

// ListLenses returns all active lenses for the user.
func (h *Handler) ListLenses(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.ListLensesRequest],
) (*connect.Response[knowledgehomev1.ListLensesResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	result, err := h.listLensesUsecase.Execute(ctx, user.UserID)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ListLenses")
	}

	protoLenses := make([]*knowledgehomev1.Lens, 0, len(result.Lenses))
	for _, l := range result.Lenses {
		protoLenses = append(protoLenses, convertLensToProto(l, l.CurrentVersion))
	}

	resp := &knowledgehomev1.ListLensesResponse{Lenses: protoLenses}
	if result.ActiveLensID != nil {
		activeLensID := result.ActiveLensID.String()
		resp.ActiveLensId = &activeLensID
	}
	return connect.NewResponse(resp), nil
}

// SelectLens sets the active lens for the user.
func (h *Handler) SelectLens(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.SelectLensRequest],
) (*connect.Response[knowledgehomev1.SelectLensResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagLensV0, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("lens feature is not enabled for this user"))
	}

	lensID := uuid.Nil
	if req.Msg.GetLensId() != "" {
		parsedLensID, err := parseUUID(req.Msg.GetLensId(), "lens_id")
		if err != nil {
			return nil, err
		}
		lensID = parsedLensID
	}

	if err := h.selectLensUsecase.Execute(ctx, user.UserID, lensID); err != nil {
		if errors.Is(err, domain.ErrLensNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, domain.ErrLensNotFound)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "SelectLens")
	}

	return connect.NewResponse(&knowledgehomev1.SelectLensResponse{}), nil
}
