package track_home_action_usecase

import (
	"alt/domain"
	"alt/port/feature_flag_port"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_user_event_port"
	"alt/port/recall_signal_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Valid action types.
var validActionTypes = map[string]string{
	"open":        domain.EventHomeItemOpened,
	"dismiss":     domain.EventHomeItemDismissed,
	"ask":         domain.EventHomeItemAsked,
	"listen":      domain.EventHomeItemListened,
	"open_recap":  domain.EventHomeItemOpened,
	"open_search": domain.EventHomeItemOpened,
}

// actionToSignalType maps action types that should generate recall signals.
var actionToSignalType = map[string]string{
	"open":   domain.SignalOpened,
	"ask":    domain.SignalAugurReferenced,
	"listen": domain.SignalTagInterest,
}

// TrackHomeActionUsecase records user actions on knowledge home items.
type TrackHomeActionUsecase struct {
	userEventPort      knowledge_user_event_port.AppendKnowledgeUserEventPort
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort
	featureFlagPort    feature_flag_port.FeatureFlagPort
	recallSignalPort   recall_signal_port.AppendRecallSignalPort
}

// SetRecallSignalPort wires the optional recall signal port.
func (u *TrackHomeActionUsecase) SetRecallSignalPort(port recall_signal_port.AppendRecallSignalPort) {
	u.recallSignalPort = port
}

// NewTrackHomeActionUsecase creates a new TrackHomeActionUsecase.
func NewTrackHomeActionUsecase(
	userEventPort knowledge_user_event_port.AppendKnowledgeUserEventPort,
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort,
	featureFlagPort feature_flag_port.FeatureFlagPort,
) *TrackHomeActionUsecase {
	return &TrackHomeActionUsecase{
		userEventPort:      userEventPort,
		knowledgeEventPort: knowledgeEventPort,
		featureFlagPort:    featureFlagPort,
	}
}

// Execute records a user action on a knowledge home item.
func (u *TrackHomeActionUsecase) Execute(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, actionType string, itemKey string, metadataJSON string) error {
	eventType, ok := validActionTypes[actionType]
	if !ok {
		return errors.New("invalid action type: " + actionType)
	}

	if itemKey == "" {
		return errors.New("item_key is required")
	}

	// Skip tracking if tracking flag is disabled
	if u.featureFlagPort != nil && !u.featureFlagPort.IsEnabled(domain.FlagKnowledgeHomeTracking, userID) {
		return nil
	}

	now := time.Now()

	// Record user event
	payload, _ := json.Marshal(map[string]string{
		"action_type":   actionType,
		"metadata_json": metadataJSON,
	})

	userEvent := domain.KnowledgeUserEvent{
		UserEventID: uuid.New(),
		OccurredAt:  now,
		UserID:      userID,
		TenantID:    tenantID,
		EventType:   actionType,
		ItemKey:     itemKey,
		Payload:     payload,
	}

	if err := u.userEventPort.AppendKnowledgeUserEvent(ctx, userEvent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append user action event",
			"error", err, "action_type", actionType, "item_key", itemKey)
		return fmt.Errorf("track home action: %w", err)
	}

	// Also append to knowledge_events for projector consumption
	knowledgePayload, _ := json.Marshal(map[string]string{
		"action_type": actionType,
		"item_key":    itemKey,
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"opened_at":   now.Format(time.RFC3339),
	})

	knowledgeEvent := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    now,
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     eventType,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   itemKey,
		DedupeKey:     fmt.Sprintf("%s:%s:%s:%d", userID, actionType, itemKey, now.UnixMilli()),
		Payload:       knowledgePayload,
	}

	if err := u.knowledgeEventPort.AppendKnowledgeEvent(ctx, knowledgeEvent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append knowledge event for action",
			"error", err, "action_type", actionType)
		// Non-fatal: user event was already recorded
	}

	// Append recall signal for eligible action types (non-fatal)
	if signalType, ok := actionToSignalType[actionType]; ok && u.recallSignalPort != nil {
		signal := domain.RecallSignal{
			SignalID:       uuid.New(),
			UserID:         userID,
			ItemKey:        itemKey,
			SignalType:     signalType,
			SignalStrength: 1.0,
			OccurredAt:     now,
			Payload:        map[string]any{"source": "home_action", "action_type": actionType},
		}
		if err := u.recallSignalPort.AppendRecallSignal(ctx, signal); err != nil {
			slog.ErrorContext(ctx, "failed to append recall signal",
				"error", err, "action_type", actionType, "item_key", itemKey)
		}
	}

	return nil
}
