package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// === Recall Signals ===

func (h *SovereignHandler) ListRecallSignals(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListRecallSignalsRequest],
) (*connect.Response[sovereignv1.ListRecallSignalsResponse], error) {
	userID, err := parseUUIDField("user_id", req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
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
	s, err := validateAndBuildRecallSignal(req.Msg.Signal)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.readDB.AppendRecallSignal(ctx, s); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendRecallSignal: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendRecallSignalResponse{}), nil
}

// validateAndBuildRecallSignal validates proto RecallSignal and builds domain RecallSignal.
func validateAndBuildRecallSignal(ps *sovereignv1.RecallSignal) (sovereign_db.RecallSignal, error) {
	if ps == nil {
		return sovereign_db.RecallSignal{}, fmt.Errorf("signal is required")
	}
	signalID, err := parseUUIDField("signal_id", ps.SignalId)
	if err != nil {
		return sovereign_db.RecallSignal{}, err
	}
	signalUserID, err := parseUUIDField("user_id", ps.UserId)
	if err != nil {
		return sovereign_db.RecallSignal{}, err
	}
	if ps.OccurredAt == nil {
		return sovereign_db.RecallSignal{}, fmt.Errorf("occurred_at is required")
	}
	return sovereign_db.RecallSignal{
		SignalID:       signalID,
		UserID:         signalUserID,
		ItemKey:        ps.ItemKey,
		SignalType:     ps.SignalType,
		SignalStrength: ps.SignalStrength,
		Payload:        ps.Payload,
		OccurredAt:     ps.OccurredAt.AsTime(),
	}, nil
}
