package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// clearKnowledgeHomeItemSummaryQuery clears a retracted summary excerpt from
// knowledge_home_items and resets summary_state to 'pending' while stripping
// any `summary_completed` entry from why_json and lowering unopened score back
// to baseline (0.5). Used exclusively by the SummaryRetracted projector branch
// (ADR-001042) because UpsertKnowledgeHomeItem's merge-safe GREATEST latch
// intentionally refuses to regress summary_state.
const clearKnowledgeHomeItemSummaryQuery = `
UPDATE knowledge_home_items
SET summary_excerpt = '',
    summary_state = 'pending',
    supersede_state = '',
    why_json = COALESCE((
        SELECT jsonb_agg(elem)
        FROM jsonb_array_elements(COALESCE(knowledge_home_items.why_json, '[]'::jsonb)) AS elem
        WHERE elem->>'code' <> 'summary_completed'
    ), '[]'::jsonb),
    score = CASE
        WHEN last_interacted_at IS NULL AND tags_json IS NOT NULL AND tags_json <> '[]'::jsonb THEN LEAST(score, 0.7)
        WHEN last_interacted_at IS NULL THEN LEAST(score, 0.5)
        ELSE score
    END,
    updated_at = $4
WHERE user_id = $1
  AND item_key = $2
  AND projection_version = $3
`

const clearKnowledgeHomeItemSummaryWithTenantQuery = `
UPDATE knowledge_home_items
SET summary_excerpt = '',
    summary_state = 'pending',
    supersede_state = '',
    why_json = COALESCE((
        SELECT jsonb_agg(elem)
        FROM jsonb_array_elements(COALESCE(knowledge_home_items.why_json, '[]'::jsonb)) AS elem
        WHERE elem->>'code' <> 'summary_completed'
    ), '[]'::jsonb),
    score = CASE
        WHEN last_interacted_at IS NULL AND tags_json IS NOT NULL AND tags_json <> '[]'::jsonb THEN LEAST(score, 0.7)
        WHEN last_interacted_at IS NULL THEN LEAST(score, 0.5)
        ELSE score
    END,
    updated_at = $4
WHERE user_id = $1
  AND item_key = $2
  AND projection_version = $3
  AND tenant_id = $5
`

// ClearKnowledgeHomeItemSummaryPayload is the canonical JSON wire schema for
// ClearKnowledgeHomeItemSummary (built by knowledge_home_projector's
// SummaryRetracted fold).
type ClearKnowledgeHomeItemSummaryPayload struct {
	UserID            string    `json:"user_id"`
	TenantID          string    `json:"tenant_id,omitempty"`
	ItemKey           string    `json:"item_key"`
	ProjectionVersion int       `json:"projection_version"`
	OccurredAt        time.Time `json:"occurred_at"`
}

// ClearKnowledgeHomeItemSummary clears a retracted summary from one row
// identified by (user_id, item_key, projection_version).
func (r *Repository) ClearKnowledgeHomeItemSummary(ctx context.Context, payload json.RawMessage) error {
	var p ClearKnowledgeHomeItemSummaryPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: unmarshal: %w: %w", ErrInvalidMutationPayload, err)
	}
	if p.ItemKey == "" {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: item_key is required: %w", ErrInvalidMutationPayload)
	}
	if p.OccurredAt.IsZero() {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: occurred_at is required: %w", ErrInvalidMutationPayload)
	}
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: parse user_id: %w: %w", ErrInvalidMutationPayload, err)
	}
	if userID == uuid.Nil {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: user_id cannot be nil: %w", ErrInvalidMutationPayload)
	}
	if p.TenantID != "" {
		tenantID, err := uuid.Parse(p.TenantID)
		if err != nil {
			return fmt.Errorf("ClearKnowledgeHomeItemSummary: parse tenant_id: %w: %w", ErrInvalidMutationPayload, err)
		}
		if tenantID == uuid.Nil {
			return fmt.Errorf("ClearKnowledgeHomeItemSummary: tenant_id cannot be nil: %w", ErrInvalidMutationPayload)
		}
		if _, err := r.pool.Exec(ctx, clearKnowledgeHomeItemSummaryWithTenantQuery, userID, p.ItemKey, p.ProjectionVersion, p.OccurredAt, tenantID); err != nil {
			return fmt.Errorf("ClearKnowledgeHomeItemSummary: %w", err)
		}
		return nil
	}
	if _, err := r.pool.Exec(ctx, clearKnowledgeHomeItemSummaryQuery, userID, p.ItemKey, p.ProjectionVersion, p.OccurredAt); err != nil {
		return fmt.Errorf("ClearKnowledgeHomeItemSummary: %w", err)
	}
	return nil
}
