// Package recap implements the RecapService Connect-RPC handlers.
package recap

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"

	recapv2 "alt/gen/proto/alt/recap/v2"
	"alt/gen/proto/alt/recap/v2/recapv2connect"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	recapinternal "alt/internal/recap"
)

// Handler implements the RecapService Connect-RPC service.
type Handler struct {
	recapUsecase       RecapUsecaseInterface
	clusterDraftLoader *recapinternal.ClusterDraftLoader
	logger             *slog.Logger
}

// RecapUsecaseInterface defines the interface for recap usecase.
type RecapUsecaseInterface interface {
	GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetThreeDayRecapCards(ctx context.Context) (*domain.RecapCardsResponse, error)
	GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error)
	SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error)
	SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, error)
}

// NewHandler creates a new Recap service handler.
func NewHandler(
	recapUsecase RecapUsecaseInterface,
	clusterDraftLoader *recapinternal.ClusterDraftLoader,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		recapUsecase:       recapUsecase,
		clusterDraftLoader: clusterDraftLoader,
		logger:             logger,
	}
}

// Verify interface implementation at compile time.
var _ recapv2connect.RecapServiceHandler = (*Handler)(nil)

// GetSevenDayRecap returns 7-day recap summary.
func (h *Handler) GetSevenDayRecap(
	ctx context.Context,
	req *connect.Request[recapv2.GetSevenDayRecapRequest],
) (*connect.Response[recapv2.GetSevenDayRecapResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	h.logger.InfoContext(ctx, "GetSevenDayRecap called", "user_id", userCtx.UserID)

	recap, err := h.recapUsecase.GetSevenDayRecap(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrRecapNotFound) {
			return nil, errorhandler.HandleNotFoundError(ctx, h.logger, "No 7-day recap available yet", "GetSevenDayRecap")
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetSevenDayRecap")
	}

	resp := domainToProto(recap)

	if req.Msg.GenreDraftId != nil && *req.Msg.GenreDraftId != "" && h.clusterDraftLoader != nil {
		draft, err := h.clusterDraftLoader.LoadDraft(*req.Msg.GenreDraftId)
		if err != nil {
			h.logger.WarnContext(ctx, "cluster draft loader failed", "error", err, "draft_id", *req.Msg.GenreDraftId)
		} else if draft != nil {
			resp.ClusterDraft = clusterDraftToProto(draft)
		}
	}

	return connect.NewResponse(resp), nil
}

// GetThreeDayRecap returns 3-day recap summary.
func (h *Handler) GetThreeDayRecap(
	ctx context.Context,
	req *connect.Request[recapv2.GetThreeDayRecapRequest],
) (*connect.Response[recapv2.GetThreeDayRecapResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	h.logger.InfoContext(ctx, "GetThreeDayRecap called", "user_id", userCtx.UserID)

	recap, err := h.recapUsecase.GetThreeDayRecap(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrRecapNotFound) {
			return nil, errorhandler.HandleNotFoundError(ctx, h.logger, "No 3-day recap available yet", "GetThreeDayRecap")
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetThreeDayRecap")
	}

	resp := domainToProtoThreeDays(recap)

	if req.Msg.GenreDraftId != nil && *req.Msg.GenreDraftId != "" && h.clusterDraftLoader != nil {
		draft, err := h.clusterDraftLoader.LoadDraft(*req.Msg.GenreDraftId)
		if err != nil {
			h.logger.WarnContext(ctx, "cluster draft loader failed", "error", err, "draft_id", *req.Msg.GenreDraftId)
		} else if draft != nil {
			resp.ClusterDraft = clusterDraftToProto(draft)
		}
	}

	return connect.NewResponse(resp), nil
}

// GetThreeDayRecapCards returns 3-day topic recap cards.
func (h *Handler) GetThreeDayRecapCards(
	ctx context.Context,
	_ *connect.Request[recapv2.GetThreeDayRecapCardsRequest],
) (*connect.Response[recapv2.GetThreeDayRecapCardsResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	h.logger.InfoContext(ctx, "GetThreeDayRecapCards called", "user_id", userCtx.UserID)

	cardsResp, err := h.recapUsecase.GetThreeDayRecapCards(ctx)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetThreeDayRecapCards")
	}

	resp := domainToProtoThreeDaysCards(cardsResp)
	return connect.NewResponse(resp), nil
}

// GetEveningPulse returns Evening Pulse data.
func (h *Handler) GetEveningPulse(
	ctx context.Context,
	req *connect.Request[recapv2.GetEveningPulseRequest],
) (*connect.Response[recapv2.GetEveningPulseResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	h.logger.InfoContext(ctx, "GetEveningPulse called", "user_id", userCtx.UserID)

	date := ""
	if req.Msg.Date != nil {
		date = *req.Msg.Date
	}

	pulse, err := h.recapUsecase.GetEveningPulse(ctx, date)
	if err != nil {
		if errors.Is(err, domain.ErrEveningPulseNotFound) {
			return nil, errorhandler.HandleNotFoundError(ctx, h.logger, "Evening Pulse not available", "GetEveningPulse")
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetEveningPulse")
	}

	resp := eveningPulseDomainToProto(pulse)
	return connect.NewResponse(resp), nil
}

// SearchRecapsByTag searches across completed recaps for genres matching a tag or query.
func (h *Handler) SearchRecapsByTag(
	ctx context.Context,
	req *connect.Request[recapv2.SearchRecapsByTagRequest],
) (*connect.Response[recapv2.SearchRecapsByTagResponse], error) {
	userCtx, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	query := req.Msg.GetQuery()
	tagName := req.Msg.TagName

	h.logger.InfoContext(ctx, "SearchRecapsByTag called", "user_id", userCtx.UserID, "tag_name", tagName, "query", query)

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var results []*domain.RecapSearchResult
	if query != "" {
		results, err = h.recapUsecase.SearchRecapsByQuery(ctx, query, limit)
	} else if tagName != "" {
		results, err = h.recapUsecase.SearchRecapsByTag(ctx, tagName, limit)
	} else {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one of query or tag_name is required"))
	}

	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "SearchRecapsByTag")
	}

	return connect.NewResponse(&recapv2.SearchRecapsByTagResponse{
		Results: recapSearchResultsToProto(results),
	}), nil
}
