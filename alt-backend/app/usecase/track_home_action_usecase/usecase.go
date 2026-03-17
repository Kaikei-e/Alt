package track_home_action_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_user_event_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// TrackHomeActionUsecase records user actions on knowledge home items.
type TrackHomeActionUsecase struct {
	userEventPort      knowledge_user_event_port.AppendKnowledgeUserEventPort
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort
}

// NewTrackHomeActionUsecase creates a new TrackHomeActionUsecase.
func NewTrackHomeActionUsecase(
	userEventPort knowledge_user_event_port.AppendKnowledgeUserEventPort,
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort,
) *TrackHomeActionUsecase {
	return &TrackHomeActionUsecase{
		userEventPort:      userEventPort,
		knowledgeEventPort: knowledgeEventPort,
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

	return nil
}
