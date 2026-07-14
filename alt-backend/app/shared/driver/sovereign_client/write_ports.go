package sovereign_client

import (
	"alt/domain"
	"alt/orchestrator/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UpsertKnowledgeHomeItem implements knowledge_home_port.UpsertKnowledgeHomeItemPort.
func (c *Client) UpsertKnowledgeHomeItem(ctx context.Context, item domain.KnowledgeHomeItem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("sovereign UpsertKnowledgeHomeItem marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationUpsertHomeItem,
		EntityID:     item.ItemKey,
		Payload:      payload,
	})
}

// DismissKnowledgeHomeItem implements knowledge_home_port.DismissKnowledgeHomeItemPort.
func (c *Client) DismissKnowledgeHomeItem(ctx context.Context, userID uuid.UUID, itemKey string, projectionVersion int, dismissedAt time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           itemKey,
		"projection_version": projectionVersion,
		"dismissed_at":       dismissedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("sovereign DismissKnowledgeHomeItem marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationDismissHomeItem,
		EntityID:     itemKey,
		Payload:      payload,
	})
}

// PatchKnowledgeHomeItemURL implements knowledge_home_port.PatchKnowledgeHomeItemURLPort.
// Routes to sovereign's PatchKnowledgeHomeItemURL driver via the
// ApplyProjectionMutation envelope with MutationType ==
// MutationPatchHomeItemURL. Used by the corrective ArticleUrlBackfilled
// projector branch only.
func (c *Client) PatchKnowledgeHomeItemURL(ctx context.Context, userID uuid.UUID, itemKey string, projectionVersion int, url string) error {
	if url == "" {
		return fmt.Errorf("sovereign PatchKnowledgeHomeItemURL: empty url rejected at client boundary")
	}
	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           itemKey,
		"projection_version": projectionVersion,
		"url":                url,
	})
	if err != nil {
		return fmt.Errorf("sovereign PatchKnowledgeHomeItemURL marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationPatchHomeItemURL,
		EntityID:     itemKey,
		Payload:      payload,
	})
}

// ClearSupersedeState implements knowledge_home_port.ClearSupersedeStatePort.
func (c *Client) ClearSupersedeState(ctx context.Context, userID uuid.UUID, itemKey string, projectionVersion int) error {
	payload, err := json.Marshal(map[string]any{
		"user_id":            userID.String(),
		"item_key":           itemKey,
		"projection_version": projectionVersion,
	})
	if err != nil {
		return fmt.Errorf("sovereign ClearSupersedeState marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationClearSupersede,
		EntityID:     itemKey,
		Payload:      payload,
	})
}

// UpsertTodayDigest implements today_digest_port.UpsertTodayDigestPort.
func (c *Client) UpsertTodayDigest(ctx context.Context, digest domain.TodayDigest) error {
	payload, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("sovereign UpsertTodayDigest marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationUpsertTodayDigest,
		EntityID:     fmt.Sprintf("digest:%s", digest.UserID),
		Payload:      payload,
	})
}

// UpsertRecallCandidate implements recall_candidate_port.UpsertRecallCandidatePort.
func (c *Client) UpsertRecallCandidate(ctx context.Context, candidate domain.RecallCandidate) error {
	payload, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("sovereign UpsertRecallCandidate marshal: %w", err)
	}
	return c.ApplyProjectionMutation(ctx, knowledge_sovereign_port.ProjectionMutation{
		MutationType: knowledge_sovereign_port.MutationUpsertRecallCandidate,
		EntityID:     candidate.ItemKey,
		Payload:      payload,
	})
}

// SnoozeRecallCandidate implements recall_candidate_port.SnoozeRecallCandidatePort.
// occurred_at is required by knowledge-sovereign's SnoozeRecallCandidate driver:
// recall_candidate_view is a reproject-safe projection, so updated_at must come
// from the event's own origination time rather than SQL now() (otherwise
// replaying the same mutation twice produces different rows).
func (c *Client) SnoozeRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, until time.Time, occurredAt time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"user_id":     userID.String(),
		"item_key":    itemKey,
		"until":       until.Format(time.RFC3339Nano),
		"occurred_at": occurredAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("sovereign SnoozeRecallCandidate marshal: %w", err)
	}
	return c.ApplyRecallMutation(ctx, knowledge_sovereign_port.RecallMutation{
		MutationType: knowledge_sovereign_port.MutationSnoozeCandidate,
		EntityID:     itemKey,
		Payload:      payload,
	})
}

// DismissRecallCandidate implements recall_candidate_port.DismissRecallCandidatePort.
// occurred_at is required for the same reproject-determinism reason as
// SnoozeRecallCandidate above.
func (c *Client) DismissRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, occurredAt time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"user_id":     userID.String(),
		"item_key":    itemKey,
		"occurred_at": occurredAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("sovereign DismissRecallCandidate marshal: %w", err)
	}
	return c.ApplyRecallMutation(ctx, knowledge_sovereign_port.RecallMutation{
		MutationType: knowledge_sovereign_port.MutationDismissCandidate,
		EntityID:     itemKey,
		Payload:      payload,
	})
}
