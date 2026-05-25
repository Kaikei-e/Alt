package knowledge_home

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
)

// TrackHomeItemsSeen records which items were visible on screen.
func (h *Handler) TrackHomeItemsSeen(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackHomeItemsSeenRequest],
) (*connect.Response[knowledgehomev1.TrackHomeItemsSeenResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	if len(req.Msg.ItemKeys) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("item_keys is required"))
	}

	if h.metrics != nil {
		h.metrics.TrackingReceivedTotal.Add(ctx, 1)
		h.metrics.ItemsExposed.Add(ctx, int64(len(req.Msg.ItemKeys)))
		if h.metrics.Snapshot != nil {
			h.metrics.Snapshot.RecordTrackingReceived()
			for range len(req.Msg.ItemKeys) {
				h.metrics.Snapshot.RecordItemExposed()
			}
		}
	}

	if err := h.trackSeenUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKeys, req.Msg.ExposureSessionId); err != nil {
		if h.metrics != nil {
			h.metrics.TrackingFailedTotal.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordTrackingFailed()
			}
		}
		return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeItemsSeen")
	}

	if h.metrics != nil {
		h.metrics.TrackingPersistedTotal.Add(ctx, 1)
		if h.metrics.Snapshot != nil {
			h.metrics.Snapshot.RecordTrackingPersisted()
		}
	}

	return connect.NewResponse(&knowledgehomev1.TrackHomeItemsSeenResponse{}), nil
}

// TrackHomeAction records a user action on a home item.
func (h *Handler) TrackHomeAction(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.TrackHomeActionRequest],
) (*connect.Response[knowledgehomev1.TrackHomeActionResponse], error) {
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

	var metadataJSON string
	if req.Msg.MetadataJson != nil {
		metadataJSON = *req.Msg.MetadataJson
	}

	// ADR-000913 §D-9 — TrackHomeAction absorbs the legacy
	// TrackRecallAction snooze/dismiss path. Older clients route
	// recall actions through the deprecated RPC; new clients send them
	// through here. dispatch order: snooze/dismiss → recall usecases,
	// everything else → trackActionUsecase.
	switch req.Msg.ActionType {
	case "snooze":
		if h.recallSnoozeUsecase == nil {
			return nil, connect.NewError(connect.CodeUnimplemented,
				fmt.Errorf("recall snooze is not enabled on this deployment"))
		}
		snoozeHours := 24
		if metadataJSON != "" {
			var meta map[string]any
			if err := json.Unmarshal([]byte(metadataJSON), &meta); err == nil {
				if v, ok := meta["snooze_hours"].(float64); ok && v > 0 {
					snoozeHours = int(v)
				}
			}
		}
		if err := h.recallSnoozeUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey, snoozeHours); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeAction.snooze")
		}
	case "dismiss_recall":
		if h.recallDismissUsecase == nil {
			return nil, connect.NewError(connect.CodeUnimplemented,
				fmt.Errorf("recall dismiss is not enabled on this deployment"))
		}
		if err := h.recallDismissUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ItemKey); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeAction.dismiss_recall")
		}
	default:
		if err := h.trackActionUsecase.Execute(ctx, user.UserID, user.TenantID, req.Msg.ActionType, req.Msg.ItemKey, metadataJSON); err != nil {
			return nil, errorhandler.HandleInternalError(ctx, h.logger, err, "TrackHomeAction")
		}
	}

	if h.metrics != nil {
		switch req.Msg.ActionType {
		case "open", "open_recap", "open_search":
			h.metrics.ItemsOpened.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordItemOpened()
			}
		case "dismiss":
			h.metrics.ItemsDismissed.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordItemDismissed()
			}
		}
	}

	return connect.NewResponse(&knowledgehomev1.TrackHomeActionResponse{}), nil
}
