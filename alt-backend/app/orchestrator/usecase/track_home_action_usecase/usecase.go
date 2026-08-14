package track_home_action_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/article_url_lookup_port"
	"alt/orchestrator/port/feature_flag_port"
	"alt/orchestrator/port/knowledge_home_port"
	"alt/orchestrator/port/knowledge_projection_version_port"
	"alt/orchestrator/port/knowledge_user_event_port"
	"alt/orchestrator/port/recall_signal_port"
	"alt/shared/port/knowledge_event_port"
	"alt/utils/logger"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// metadataIdempotencyKeyField is the metadata_json field through which a client
// hands us its own retry-stable key. metadata_json is the only channel the
// TrackHomeAction RPC has for one without a proto change.
const metadataIdempotencyKeyField = "idempotency_key"

// dedupeBucket is how coarsely the clock is quantised when the client supplies
// no idempotency key. A retry has to land in the same bucket as the attempt it
// repeats to be recognised as a retry, so the width has to exceed any realistic
// retry delay; it also bounds how long two byte-identical repeats of an action
// collapse into one event. The sibling impression path
// (track_home_seen_usecase) buckets at 5 minutes, but actions are far rarer and
// each carries more signal, so this is deliberately tighter.
const dedupeBucket = time.Minute

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

// buildDedupeKey derives the at-least-once key that both appends of one action
// share. sovereign rejects an empty dedupe_key outright: the value gates a
// partial unique index conditioned on the key being non-empty, so an empty one
// would silently disable dedup rather than fail.
//
// The key has to survive a retry. The knowledge_events append below is fatal,
// so Execute can return an error with the user event already committed, and the
// caller's only recovery is to re-issue the same action — append-first buys
// idempotency from the dedupe registry, which only collapses the second append
// when the key is byte-identical. A key carrying now.UnixMilli() cannot do
// that: the retry lands on a new millisecond, so it stacks a duplicate row.
// The same resolution failed in the other direction too — two genuinely
// distinct actions issued inside one millisecond collided and the second was
// deduped away.
//
// A client-supplied idempotency key is stable by construction, so it decides
// the key whenever one is present. Otherwise the clock is quantised into a
// coarse bucket and combined with a fingerprint of the action's metadata: a
// retry inside the bucket collapses, while two different actions on one item
// (two tags clicked, two searches run) keep distinct keys. The residual hole is
// a retry that straddles a bucket boundary, which is bounded and far narrower
// than "every retry duplicates".
func buildDedupeKey(userID uuid.UUID, actionType string, itemKey string, metadataJSON string, now time.Time) string {
	prefix := fmt.Sprintf("%s:%s:%s", userID, actionType, itemKey)

	// Still namespaced by the action tuple, so a client that reuses one key by
	// mistake collapses only its own repeats of that exact action.
	if clientKey := clientIdempotencyKey(metadataJSON); clientKey != "" {
		return prefix + ":" + clientKey
	}

	bucket := now.UTC().Truncate(dedupeBucket).Format(time.RFC3339)
	return fmt.Sprintf("%s:%s:%s", prefix, bucket, metadataFingerprint(metadataJSON))
}

// clientIdempotencyKey pulls the caller-supplied retry key out of metadata_json.
// Metadata that is absent, malformed, or carries no key is not an error here:
// the field is optional and free-form, and the time bucket still yields a
// usable key.
func clientIdempotencyKey(metadataJSON string) string {
	if metadataJSON == "" {
		return ""
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
		return ""
	}
	key, _ := meta[metadataIdempotencyKeyField].(string)
	return key
}

// metadataFingerprint keeps two different actions on one item inside the same
// bucket apart — clicking tag "rust" then tag "go" is two facts, not a retry.
// A retry replays the same serialized metadata byte for byte, so hashing the
// raw string is enough and avoids re-encoding differences of our own making.
func metadataFingerprint(metadataJSON string) string {
	sum := sha256.Sum256([]byte(metadataJSON))
	return hex.EncodeToString(sum[:8])
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

	// One action is one fact, so both appends below carry the same key.
	dedupeKey := buildDedupeKey(userID, actionType, itemKey, metadataJSON, now)

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
		DedupeKey:   dedupeKey,
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
				source, lookupErr := u.articleURLLookupPort.LookupArticleSource(ctx, articleID, userID)
				if lookupErr == nil {
					foundURL = source.URL
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
		DedupeKey:     dedupeKey,
		Payload:       knowledgePayload,
	}

	// Fatal, and deliberately ahead of every projection write below.
	// knowledge_events is the only durable record of the action: the read
	// models it feeds (knowledge_home_items in particular) are TRUNCATEd and
	// replayed by RebuildProjection. Writing dismissed_at through to the
	// projection after a failed append would survive only until the next
	// reproject, at which point the dismissed item returns to the Home rail
	// while the caller was told the action succeeded. Failing here instead
	// lets the client retry the whole action.
	if _, err := u.knowledgeEventPort.AppendKnowledgeEvent(ctx, knowledgeEvent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append knowledge event for action",
			"error", err, "action_type", actionType, "item_key", itemKey)
		return fmt.Errorf("track home action: append knowledge event: %w", err)
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
