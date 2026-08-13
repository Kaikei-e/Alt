package track_home_seen_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/feature_flag_port"
	"alt/orchestrator/port/knowledge_user_event_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TrackHomeSeenUsecase records impression events for knowledge home items.
type TrackHomeSeenUsecase struct {
	userEventPort   knowledge_user_event_port.AppendKnowledgeUserEventPort
	featureFlagPort feature_flag_port.FeatureFlagPort
}

// NewTrackHomeSeenUsecase creates a new TrackHomeSeenUsecase.
func NewTrackHomeSeenUsecase(
	userEventPort knowledge_user_event_port.AppendKnowledgeUserEventPort,
	featureFlagPort feature_flag_port.FeatureFlagPort,
) *TrackHomeSeenUsecase {
	return &TrackHomeSeenUsecase{
		userEventPort:   userEventPort,
		featureFlagPort: featureFlagPort,
	}
}

// Execute records that items were seen on the knowledge home.
func (u *TrackHomeSeenUsecase) Execute(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, itemKeys []string, exposureSessionID string) error {
	if len(itemKeys) == 0 {
		return nil
	}

	// Skip tracking if tracking flag is disabled
	if u.featureFlagPort != nil && !u.featureFlagPort.IsEnabled(domain.FlagKnowledgeHomeTracking, userID) {
		return nil
	}

	now := time.Now()
	// 5-minute bucket for deduplication
	bucket := now.Truncate(5 * time.Minute).Format(time.RFC3339)

	// Impression events have no outbox and no DLQ, so a dropped append is lost
	// for good. Every item is still attempted, but the failures travel back to
	// the caller so the RPC fails and the persisted/failed counters stay honest.
	var failures []error

	for _, itemKey := range itemKeys {
		dedupeKey := fmt.Sprintf("%s:%s:seen:%s", userID, itemKey, bucket)
		payload, _ := json.Marshal(map[string]string{
			"exposure_session_id": exposureSessionID,
		})

		event := domain.KnowledgeUserEvent{
			UserEventID: uuid.New(),
			OccurredAt:  now,
			UserID:      userID,
			TenantID:    tenantID,
			EventType:   domain.EventHomeItemsSeen,
			ItemKey:     itemKey,
			Payload:     payload,
			DedupeKey:   dedupeKey,
		}

		if err := u.userEventPort.AppendKnowledgeUserEvent(ctx, event); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to append seen event",
				"error", err, "item_key", itemKey)
			failures = append(failures, fmt.Errorf("append seen event for %s: %w", itemKey, err))
		}
	}

	return errors.Join(failures...)
}
