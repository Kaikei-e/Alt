package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// AppendRecallSignal implements recall_signal_port.AppendRecallSignalPort.
func (c *Client) AppendRecallSignal(ctx context.Context, signal domain.RecallSignal) error {
	if !c.enabled {
		return nil
	}

	payloadBytes, err := json.Marshal(signal.Payload)
	if err != nil {
		return fmt.Errorf("sovereign AppendRecallSignal marshal payload: %w", err)
	}

	_, err = c.client.AppendRecallSignal(ctx, connect.NewRequest(&sovereignv1.AppendRecallSignalRpcRequest{
		Signal: &sovereignv1.RecallSignal{
			SignalId:       signal.SignalID.String(),
			UserId:         signal.UserID.String(),
			ItemKey:        signal.ItemKey,
			SignalType:     signal.SignalType,
			SignalStrength: signal.SignalStrength,
			OccurredAt:     timeToProto(signal.OccurredAt),
			Payload:        payloadBytes,
		},
	}))
	if err != nil {
		return fmt.Errorf("sovereign AppendRecallSignal: %w", err)
	}
	return nil
}

// ListRecallSignalsByUser implements recall_signal_port.ListRecallSignalsByUserPort.
func (c *Client) ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]domain.RecallSignal, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.ListRecallSignals(ctx, connect.NewRequest(&sovereignv1.ListRecallSignalsRequest{
		UserId:    userID.String(),
		SinceDays: int32(sinceDays),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListRecallSignalsByUser: %w", err)
	}

	signals := make([]domain.RecallSignal, len(resp.Msg.Signals))
	for i, pb := range resp.Msg.Signals {
		var payload map[string]any
		if len(pb.Payload) > 0 {
			_ = json.Unmarshal(pb.Payload, &payload)
		}
		sig := domain.RecallSignal{
			SignalID:       parseUUID(pb.SignalId),
			UserID:         parseUUID(pb.UserId),
			ItemKey:        pb.ItemKey,
			SignalType:     pb.SignalType,
			SignalStrength: pb.SignalStrength,
			Payload:        payload,
		}
		if pb.OccurredAt != nil {
			sig.OccurredAt = pb.OccurredAt.AsTime()
		}
		signals[i] = sig
	}
	return signals, nil
}

// AppendKnowledgeUserEvent implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
func (c *Client) AppendKnowledgeUserEvent(ctx context.Context, event domain.KnowledgeUserEvent) error {
	if !c.enabled {
		return nil
	}

	_, err := c.client.AppendKnowledgeUserEvent(ctx, connect.NewRequest(&sovereignv1.AppendKnowledgeUserEventRequest{
		Event: &sovereignv1.KnowledgeUserEvent{
			UserEventId: event.UserEventID.String(),
			OccurredAt:  timeToProto(event.OccurredAt),
			UserId:      event.UserID.String(),
			TenantId:    event.TenantID.String(),
			EventType:   event.EventType,
			ItemKey:     event.ItemKey,
			Payload:     event.Payload,
			DedupeKey:   event.DedupeKey,
		},
	}))
	if err != nil {
		return fmt.Errorf("sovereign AppendKnowledgeUserEvent: %w", err)
	}
	return nil
}
