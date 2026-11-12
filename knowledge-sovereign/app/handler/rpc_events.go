package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// MaxKnowledgeEventPayloadBytes bounds AppendKnowledgeEvent JSON payloads (64 KiB).
const MaxKnowledgeEventPayloadBytes = 64 * 1024

// === Knowledge Events RPCs ===

func (h *SovereignHandler) ListKnowledgeEvents(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListKnowledgeEventsRequest],
) (*connect.Response[sovereignv1.ListKnowledgeEventsResponse], error) {
	msg := req.Msg
	var events []sovereign_db.KnowledgeEvent
	var err error

	if msg.UserId != "" {
		if msg.TenantId == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("tenant_id is required when user_id is set"))
		}
		userID, parseErr := uuid.Parse(msg.UserId)
		if parseErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid user_id: %w", parseErr))
		}
		tenantID, parseErr := uuid.Parse(msg.TenantId)
		if parseErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid tenant_id: %w", parseErr))
		}
		events, err = h.readDB.ListKnowledgeEventsSinceForUser(ctx, tenantID, userID, msg.AfterSeq, clampReadLimit(int(msg.Limit), defaultReadLimit))
	} else {
		events, err = h.readDB.ListKnowledgeEventsSince(ctx, msg.AfterSeq, clampReadLimit(int(msg.Limit), defaultReadLimit))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListKnowledgeEvents: %w", err))
	}

	pbEvents := make([]*sovereignv1.KnowledgeEvent, len(events))
	for i, e := range events {
		pbEvents[i] = eventToProto(e)
	}
	return connect.NewResponse(&sovereignv1.ListKnowledgeEventsResponse{Events: pbEvents}), nil
}

func (h *SovereignHandler) GetLatestEventSeq(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetLatestEventSeqRequest],
) (*connect.Response[sovereignv1.GetLatestEventSeqResponse], error) {
	if req.Msg.TenantId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("tenant_id is required"))
	}
	userID, err := uuid.Parse(req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid user_id: %w", err))
	}
	tenantID, err := uuid.Parse(req.Msg.TenantId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid tenant_id: %w", err))
	}
	seq, err := h.readDB.GetLatestKnowledgeEventSeqForUser(ctx, tenantID, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetLatestEventSeq: %w", err))
	}
	return connect.NewResponse(&sovereignv1.GetLatestEventSeqResponse{EventSeq: seq}), nil
}

func (h *SovereignHandler) AppendKnowledgeEvent(
	ctx context.Context,
	req *connect.Request[sovereignv1.AppendKnowledgeEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeEventResponse], error) {
	event, err := validateAndBuildKnowledgeEvent(req.Msg.Event)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	seq, err := h.readDB.AppendKnowledgeEvent(ctx, event)
	if err != nil {
		if errors.Is(err, sovereign_db.ErrDedupeOwnerUnresolved) {
			slog.ErrorContext(ctx, "append knowledge event dedupe owner unresolved",
				"event_type", event.EventType,
				"dedupe_key_sha256", dedupeKeySHA256Prefix(event.DedupeKey),
			)
			return nil, connect.NewError(connect.CodeUnavailable, sovereign_db.ErrDedupeOwnerUnresolved)
		}
		if errors.Is(err, sovereign_db.ErrDedupeKeyTenantMismatch) {
			slog.WarnContext(ctx, "append knowledge event dedupe tenant mismatch",
				"event_type", event.EventType,
				"dedupe_key_sha256", dedupeKeySHA256Prefix(event.DedupeKey),
			)
			return nil, connect.NewError(connect.CodePermissionDenied, sovereign_db.ErrDedupeKeyTenantMismatch)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendKnowledgeEvent: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendKnowledgeEventResponse{EventSeq: seq}), nil
}

func dedupeKeySHA256Prefix(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func (h *SovereignHandler) AppendKnowledgeUserEvent(
	ctx context.Context,
	req *connect.Request[sovereignv1.AppendKnowledgeUserEventRequest],
) (*connect.Response[sovereignv1.AppendKnowledgeUserEventResponse], error) {
	event, err := validateAndBuildKnowledgeUserEvent(req.Msg.Event)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := h.readDB.AppendKnowledgeUserEvent(ctx, event); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("AppendKnowledgeUserEvent: %w", err))
	}
	return connect.NewResponse(&sovereignv1.AppendKnowledgeUserEventResponse{}), nil
}

// === Pure validators and mappers ===

func validateAndBuildKnowledgeEvent(pe *sovereignv1.KnowledgeEvent) (sovereign_db.KnowledgeEvent, error) {
	if pe == nil {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("event is required")
	}

	eventID, err := parseUUIDField("event_id", pe.EventId)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}
	tenantID, err := parseUUIDField("tenant_id", pe.TenantId)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}
	if tenantID == uuid.Nil {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("tenant_id must not be nil UUID")
	}
	userID, err := parseUUIDPtrField("user_id", pe.UserId)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}
	if userID != nil && *userID == uuid.Nil {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("user_id must not be nil UUID")
	}
	correlationID, err := parseUUIDPtrField("correlation_id", pe.CorrelationId)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}
	causationID, err := parseUUIDPtrField("causation_id", pe.CausationId)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}

	if pe.OccurredAt == nil {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("occurred_at is required")
	}
	if strings.TrimSpace(pe.DedupeKey) == "" {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("dedupe_key is required")
	}
	if err := validateKnowledgeEventPayload(pe.Payload); err != nil {
		return sovereign_db.KnowledgeEvent{}, err
	}

	return sovereign_db.KnowledgeEvent{
		EventID:       eventID,
		ActorType:     pe.ActorType,
		ActorID:       pe.ActorId,
		EventType:     pe.EventType,
		AggregateType: pe.AggregateType,
		AggregateID:   pe.AggregateId,
		DedupeKey:     pe.DedupeKey,
		Payload:       json.RawMessage(pe.Payload),
		TenantID:      tenantID,
		UserID:        userID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    pe.OccurredAt.AsTime(),
	}, nil
}

func validateKnowledgeEventPayload(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("payload is required")
	}
	if len(raw) > MaxKnowledgeEventPayloadBytes {
		return fmt.Errorf("payload exceeds maximum size of %d bytes", MaxKnowledgeEventPayloadBytes)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("payload must be valid UTF-8")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return fmt.Errorf("payload must be a valid JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return fmt.Errorf("payload must be a valid JSON object")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("payload must contain a single JSON object")
	}
	return nil
}

func validateAndBuildKnowledgeUserEvent(pe *sovereignv1.KnowledgeUserEvent) (sovereign_db.KnowledgeUserEvent, error) {
	if pe == nil {
		return sovereign_db.KnowledgeUserEvent{}, fmt.Errorf("event is required")
	}
	userEventID, err := parseUUIDField("user_event_id", pe.UserEventId)
	if err != nil {
		return sovereign_db.KnowledgeUserEvent{}, err
	}
	eventUserID, err := parseUUIDField("user_id", pe.UserId)
	if err != nil {
		return sovereign_db.KnowledgeUserEvent{}, err
	}
	eventTenantID, err := parseUUIDField("tenant_id", pe.TenantId)
	if err != nil {
		return sovereign_db.KnowledgeUserEvent{}, err
	}
	if pe.OccurredAt == nil {
		return sovereign_db.KnowledgeUserEvent{}, fmt.Errorf("occurred_at is required")
	}
	// dedupe_key gates the driver's `WHERE dedupe_key != ''` partial unique
	// index (read_events.go AppendKnowledgeUserEvent) — an empty value
	// silently disables at-least-once dedup instead of failing, so it must
	// be required here rather than left to the caller's discretion.
	if strings.TrimSpace(pe.DedupeKey) == "" {
		return sovereign_db.KnowledgeUserEvent{}, fmt.Errorf("dedupe_key is required")
	}
	return sovereign_db.KnowledgeUserEvent{
		UserEventID: userEventID,
		UserID:      eventUserID,
		TenantID:    eventTenantID,
		EventType:   pe.EventType,
		ItemKey:     pe.ItemKey,
		Payload:     pe.Payload,
		DedupeKey:   pe.DedupeKey,
		OccurredAt:  pe.OccurredAt.AsTime(),
	}, nil
}

func eventToProto(e sovereign_db.KnowledgeEvent) *sovereignv1.KnowledgeEvent {
	pb := &sovereignv1.KnowledgeEvent{
		EventId:       e.EventID.String(),
		EventSeq:      e.EventSeq,
		OccurredAt:    timestamppb.New(e.OccurredAt),
		TenantId:      e.TenantID.String(),
		ActorType:     e.ActorType,
		ActorId:       e.ActorID,
		EventType:     e.EventType,
		AggregateType: e.AggregateType,
		AggregateId:   e.AggregateID,
		DedupeKey:     e.DedupeKey,
		Payload:       e.Payload,
	}
	if e.UserID != nil {
		pb.UserId = e.UserID.String()
	}
	if e.CorrelationID != nil {
		pb.CorrelationId = e.CorrelationID.String()
	}
	if e.CausationID != nil {
		pb.CausationId = e.CausationID.String()
	}
	return pb
}
