package track_home_action_usecase

import (
	"alt/domain"
	"alt/port/article_url_lookup_port"
	"alt/port/feature_flag_port"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_home_port"
	"alt/port/knowledge_projection_version_port"
	"alt/port/knowledge_user_event_port"
	"alt/port/recall_signal_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// articleItemKeyPrefix marks an item_key whose payload anchors back to an
// `articles` row (item_key = "article:<uuid>"). Used to gate the article-URL
// payload enrichment so non-article home items skip the lookup entirely.
const articleItemKeyPrefix = "article:"

// Valid action types.
var validActionTypes = map[string]string{
	"open":        domain.EventHomeItemOpened,
	"dismiss":     domain.EventHomeItemDismissed,
	"ask":         domain.EventHomeItemAsked,
	"listen":      domain.EventHomeItemListened,
	"open_recap":  domain.EventHomeItemOpened,
	"open_search": domain.EventHomeItemOpened,
	"tag_click":   domain.EventHomeItemTagClicked,
}

// actionToSignalType maps action types that should generate recall signals.
var actionToSignalType = map[string]string{
	"open":        domain.SignalOpened,
	"ask":         domain.SignalAugurReferenced,
	"listen":      domain.SignalTagInterest,
	"open_search": domain.SignalSearchRelated,
	"tag_click":   domain.SignalTagClicked,
}

// TrackHomeActionUsecase records user actions on knowledge home items.
type TrackHomeActionUsecase struct {
	userEventPort        knowledge_user_event_port.AppendKnowledgeUserEventPort
	knowledgeEventPort   knowledge_event_port.AppendKnowledgeEventPort
	featureFlagPort      feature_flag_port.FeatureFlagPort
	recallSignalPort     recall_signal_port.AppendRecallSignalPort
	dismissPort          knowledge_home_port.DismissKnowledgeHomeItemPort
	activeVersionPort    knowledge_projection_version_port.GetActiveVersionPort
	articleURLLookupPort article_url_lookup_port.ArticleURLLookupPort
}

// NewTrackHomeActionUsecase creates a new TrackHomeActionUsecase.
//
// articleURLLookupPort is optional (may be nil). When supplied, the usecase
// resolves article-anchored item_keys to their canonical source URL at
// append time and threads it into the knowledge_events payload so the
// downstream Knowledge Loop projector can stay reproject-safe and never read
// the latest article state.
func NewTrackHomeActionUsecase(
	userEventPort knowledge_user_event_port.AppendKnowledgeUserEventPort,
	knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort,
	featureFlagPort feature_flag_port.FeatureFlagPort,
	recallSignalPort recall_signal_port.AppendRecallSignalPort,
	dismissPort knowledge_home_port.DismissKnowledgeHomeItemPort,
	activeVersionPort knowledge_projection_version_port.GetActiveVersionPort,
	articleURLLookupPort article_url_lookup_port.ArticleURLLookupPort,
) *TrackHomeActionUsecase {
	return &TrackHomeActionUsecase{
		userEventPort:        userEventPort,
		knowledgeEventPort:   knowledgeEventPort,
		featureFlagPort:      featureFlagPort,
		recallSignalPort:     recallSignalPort,
		dismissPort:          dismissPort,
		activeVersionPort:    activeVersionPort,
		articleURLLookupPort: articleURLLookupPort,
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

	// Skip tracking if tracking flag is disabled, but always allow dismiss
	if u.featureFlagPort != nil && !u.featureFlagPort.IsEnabled(domain.FlagKnowledgeHomeTracking, userID) {
		if actionType != "dismiss" {
			return nil
		}
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

	// Also append to knowledge_events for projector consumption.
	//
	// For article-anchored item_keys we resolve the source URL once here so
	// the projector can copy it onto act_targets[].source_url without doing
	// its own state lookup (reproject-safe). Lookup failures are non-fatal:
	// we log article_id + error (URL body intentionally NOT logged — see
	// security audit Low #6) and proceed with an empty URL so legacy /
	// missing rows degrade gracefully.
	knowledgePayloadFields := map[string]string{
		"action_type": actionType,
		"item_key":    itemKey,
		"user_id":     userID.String(),
		"tenant_id":   tenantID.String(),
		"opened_at":   now.Format(time.RFC3339),
	}
	if u.articleURLLookupPort != nil && strings.HasPrefix(itemKey, articleItemKeyPrefix) {
		articleID := strings.TrimPrefix(itemKey, articleItemKeyPrefix)
		if _, parseErr := uuid.Parse(articleID); parseErr != nil {
			logger.Logger.WarnContext(ctx, "skipping article URL lookup: malformed article id",
				"article_id", articleID)
		} else {
			// Plan: Knowledge Loop 体験回復 — Pillar 2C. Retry transient lookup
			// failures up to 3 times with a 100ms backoff. The append below
			// stays unconditional (append-first invariant): if every retry
			// fails, the event is still appended without a `url` key, and the
			// long-term self-heal lives in the ArticleUrlBackfilled corrective
			// projector path. Suppressing the append on lookup failure would
			// silently drop user actions from the event log — explicitly
			// rejected by immutable-design-guard.
			const maxAttempts = 3
			const backoff = 100 * time.Millisecond
			var foundURL string
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				url, lookupErr := u.articleURLLookupPort.LookupArticleURL(ctx, articleID, userID)
				if lookupErr == nil {
					foundURL = url
					break
				}
				if attempt == maxAttempts {
					logger.Logger.WarnContext(ctx, "lookup_article_url failed after retries",
						"article_id", articleID, "attempts", maxAttempts, "error", lookupErr)
					break
				}
				select {
				case <-ctx.Done():
					logger.Logger.WarnContext(ctx, "lookup_article_url cancelled mid-retry",
						"article_id", articleID, "attempt", attempt)
					attempt = maxAttempts // exit loop without further sleep
				case <-time.After(backoff):
					// next attempt
				}
			}
			if foundURL != "" {
				knowledgePayloadFields["url"] = foundURL
			}
		}
	}
	knowledgePayload, _ := json.Marshal(knowledgePayloadFields)

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

	if _, err := u.knowledgeEventPort.AppendKnowledgeEvent(ctx, knowledgeEvent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append knowledge event for action",
			"error", err, "action_type", actionType)
		// Non-fatal: user event was already recorded
	}

	if actionType == "dismiss" && u.dismissPort != nil {
		projectionVersion := 1
		if u.activeVersionPort != nil {
			v, err := u.activeVersionPort.GetActiveVersion(ctx)
			if err != nil {
				logger.Logger.WarnContext(ctx, "failed to resolve active projection version for dismiss write-through",
					"error", err, "item_key", itemKey)
			} else if v != nil {
				projectionVersion = v.Version
			}
		}

		if err := u.dismissPort.DismissKnowledgeHomeItem(ctx, userID, itemKey, projectionVersion, now); err != nil {
			if errors.Is(err, knowledge_home_port.ErrDismissTargetNotFound) {
				logger.Logger.WarnContext(ctx, "dismiss write-through skipped because read model target was not found",
					"item_key", itemKey, "projection_version", projectionVersion)
			} else {
				logger.Logger.ErrorContext(ctx, "failed to dismiss read model synchronously",
					"error", err, "item_key", itemKey, "projection_version", projectionVersion)
			}
		}
	}

	// Append recall signal for eligible action types (non-fatal)
	if signalType, ok := actionToSignalType[actionType]; ok && u.recallSignalPort != nil {
		signalPayload := map[string]any{"source": "home_action", "action_type": actionType}
		if metadataJSON != "" {
			var meta map[string]any
			if err := json.Unmarshal([]byte(metadataJSON), &meta); err == nil {
				if q, ok := meta["query"].(string); ok && q != "" {
					signalPayload["search_query"] = q
				}
				if t, ok := meta["tag"].(string); ok && t != "" {
					signalPayload["tag"] = t
				}
			}
		}
		signal := domain.RecallSignal{
			SignalID:       uuid.New(),
			UserID:         userID,
			ItemKey:        itemKey,
			SignalType:     signalType,
			SignalStrength: 1.0,
			OccurredAt:     now,
			Payload:        signalPayload,
		}
		if err := u.recallSignalPort.AppendRecallSignal(ctx, signal); err != nil {
			slog.ErrorContext(ctx, "failed to append recall signal",
				"error", err, "action_type", actionType, "item_key", itemKey)
		}
	}

	return nil
}
