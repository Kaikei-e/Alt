package knowledge_home

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
)

// GetRecallRail returns recall candidates for the user.
func (h *Handler) GetRecallRail(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetRecallRailRequest],
) (*connect.Response[knowledgehomev1.GetRecallRailResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagRecallRail, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("recall rail is not enabled for this user"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 5
	}

	candidates, err := h.recallRailUsecase.Execute(ctx, user.UserID, limit)
	if err != nil {
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "GetRecallRail")
	}

	protoCandidates := make([]*knowledgehomev1.RecallCandidate, 0, len(candidates))
	for _, c := range candidates {
		protoCandidates = append(protoCandidates, convertRecallCandidateToProto(c))
	}

	return connect.NewResponse(&knowledgehomev1.GetRecallRailResponse{
		Candidates: protoCandidates,
	}), nil
}

// TrackRecallAction records a recall action (snooze/dismiss/open).
func (h *Handler) TrackRecallAction(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackRecallActionRequest],
) (*connect.Response[knowledgehomev1.TrackRecallActionResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if req.Msg.ActionType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("action_type is required"))
	}
	if req.Msg.ItemKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item_key is required"))
	}

	switch req.Msg.ActionType {
	case "snooze":
		snoozeHours := 24
		if req.Msg.SnoozeHours != nil {
			snoozeHours = int(*req.Msg.SnoozeHours)
		}
		if err := h.recallSnoozeUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey, snoozeHours); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.snooze")
		}
	case "dismiss":
		if err := h.recallDismissUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.dismiss")
		}
	case "open":
		// Track as a regular home action
		if err := h.trackActionUsecase.Execute(ctx, user.UserID, user.TenantID, "open", req.Msg.ItemKey, ""); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackRecallAction.open")
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown action_type: %s", req.Msg.ActionType))
	}

	return connect.NewResponse(&knowledgehomev1.TrackRecallActionResponse{}), nil
}

// StreamRecallRailUpdates streams real-time updates for the recall rail.
func (h *Handler) StreamRecallRailUpdates(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.StreamRecallRailUpdatesRequest],
	stream *connect.ServerStream[knowledgehomev1.StreamRecallRailUpdatesResponse],
) error {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagRecallRail, user.UserID) {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("recall rail is not enabled for this user"))
	}

	h.logger.InfoContext(ctx, "alt.recall_rail.stream_started",
		"user_id", user.UserID)

	// Send immediate heartbeat so the client receives the first byte instantly.
	if err := stream.Send(&knowledgehomev1.StreamRecallRailUpdatesResponse{
		EventType:  "heartbeat",
		OccurredAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		return err
	}

	updateTicker := time.NewTicker(30 * time.Second)
	defer updateTicker.Stop()

	heartbeatTicker := time.NewTicker(10 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-updateTicker.C:
			if h.recallRailUsecase == nil {
				continue
			}
			candidates, err := h.recallRailUsecase.Execute(ctx, user.UserID, 5)
			if err != nil {
				h.logger.ErrorContext(ctx, "recall stream: failed to fetch candidates", "error", err)
				continue
			}
			for _, c := range candidates {
				update := &knowledgehomev1.StreamRecallRailUpdatesResponse{
					EventType:  "candidate_updated",
					Candidate:  convertRecallCandidateToProto(c),
					OccurredAt: time.Now().Format(time.RFC3339),
				}
				if err := stream.Send(update); err != nil {
					return err
				}
			}

		case <-heartbeatTicker.C:
			if err := stream.Send(&knowledgehomev1.StreamRecallRailUpdatesResponse{
				EventType:  "heartbeat",
				OccurredAt: time.Now().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
}
