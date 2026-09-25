package datahubapi

import (
	"context"
	"errors"
	"time"

	"alt/dataplane/usecase/outbox_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
)

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// ClaimOutboxBatch takes ownership of pending events.
//
// The response reports the events as PROCESSING because that is what they are
// by the time the transaction commits. Reporting the status the rows had when
// the SELECT matched them would describe a state no other caller can observe.
func (h *Handler) ClaimOutboxBatch(ctx context.Context, req *connect.Request[datahubv1.ClaimOutboxBatchRequest]) (*connect.Response[datahubv1.ClaimOutboxBatchResponse], error) {
	events, err := h.outboxUsecase.ClaimBatch(ctx, int(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ClaimOutboxBatch failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to claim outbox batch"))
	}

	out := make([]*datahubv1.OutboxEvent, 0, len(events))
	for _, e := range events {
		out = append(out, &datahubv1.OutboxEvent{
			Id:        e.ID,
			EventType: e.EventType,
			Payload:   e.Payload,
			Status:    outboxStatusToProto(e.Status),
			CreatedAt: timestampOrNil(e.CreatedAt),
		})
	}
	return connect.NewResponse(&datahubv1.ClaimOutboxBatchResponse{Events: out}), nil
}

// MarkOutboxProcessed records a terminal outcome.
//
// A non-terminal status is InvalidArgument, not a quiet write: the caller
// asked for a transition this procedure does not own, and answering OK would
// leave the row in whatever state it was already in while the caller believed
// it had moved.
func (h *Handler) MarkOutboxProcessed(ctx context.Context, req *connect.Request[datahubv1.MarkOutboxProcessedRequest]) (*connect.Response[datahubv1.MarkOutboxProcessedResponse], error) {
	status := outboxStatusFromProto(req.Msg.GetStatus())

	err := h.outboxUsecase.MarkProcessed(ctx, req.Msg.GetId(), status, req.Msg.GetErrorMessage())
	switch {
	case errors.Is(err, outbox_usecase.ErrNotTerminalStatus), errors.Is(err, outbox_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "MarkOutboxProcessed failed", "error", err, "event_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark outbox event processed"))
	}

	return connect.NewResponse(&datahubv1.MarkOutboxProcessedResponse{}), nil
}

// ReleaseOutboxEvent returns a claimed-but-unattempted event to PENDING.
func (h *Handler) ReleaseOutboxEvent(ctx context.Context, req *connect.Request[datahubv1.ReleaseOutboxEventRequest]) (*connect.Response[datahubv1.ReleaseOutboxEventResponse], error) {
	err := h.outboxUsecase.Release(ctx, req.Msg.GetId())
	switch {
	case errors.Is(err, outbox_usecase.ErrMissingID):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "ReleaseOutboxEvent failed", "error", err, "event_id", req.Msg.GetId())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to release outbox event"))
	}

	return connect.NewResponse(&datahubv1.ReleaseOutboxEventResponse{}), nil
}

// PruneOutboxEvents deletes PROCESSED rows past the caller's retention window.
func (h *Handler) PruneOutboxEvents(ctx context.Context, req *connect.Request[datahubv1.PruneOutboxEventsRequest]) (*connect.Response[datahubv1.PruneOutboxEventsResponse], error) {
	retention := time.Duration(req.Msg.GetOlderThanSeconds()) * time.Second

	pruned, err := h.outboxUsecase.Prune(ctx, retention)
	switch {
	case errors.Is(err, outbox_usecase.ErrInvalidRetention):
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	case err != nil:
		h.logger.ErrorContext(ctx, "PruneOutboxEvents failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to prune outbox events"))
	}

	return connect.NewResponse(&datahubv1.PruneOutboxEventsResponse{PrunedCount: pruned}), nil
}

func outboxStatusToProto(s domain.OutboxEventStatus) datahubv1.OutboxEventStatus {
	switch s {
	case domain.OutboxPending:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PENDING
	case domain.OutboxProcessing:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING
	case domain.OutboxProcessed:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSED
	case domain.OutboxFailed:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_FAILED
	default:
		return datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_UNSPECIFIED
	}
}

// outboxStatusFromProto maps an unrecognised or unspecified enum to the empty
// status, which the usecase rejects as non-terminal. Defaulting to PROCESSED
// here would turn "the caller sent a field this build does not know" into "the
// event was delivered".
func outboxStatusFromProto(s datahubv1.OutboxEventStatus) domain.OutboxEventStatus {
	switch s {
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PENDING:
		return domain.OutboxPending
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING:
		return domain.OutboxProcessing
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSED:
		return domain.OutboxProcessed
	case datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_FAILED:
		return domain.OutboxFailed
	default:
		return ""
	}
}
