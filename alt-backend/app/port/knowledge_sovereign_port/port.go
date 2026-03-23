package knowledge_sovereign_port

import (
	"context"
	"encoding/json"
	"fmt"
)

// Projection mutation types.
const (
	MutationUpsertHomeItem        = "upsert_home_item"
	MutationDismissHomeItem       = "dismiss_home_item"
	MutationClearSupersede        = "clear_supersede"
	MutationUpsertTodayDigest     = "upsert_today_digest"
	MutationUpsertRecallCandidate = "upsert_recall_candidate"
)

// Curation mutation types.
const (
	MutationDismissCuration   = "dismiss_curation"
	MutationCreateLens        = "create_lens"
	MutationCreateLensVersion = "create_lens_version"
	MutationSelectLens        = "select_lens"
	MutationClearLens         = "clear_lens"
	MutationArchiveLens       = "archive_lens"
)

// Recall mutation types.
const (
	MutationUpsertCandidate  = "upsert_candidate"
	MutationSnoozeCandidate  = "snooze_candidate"
	MutationDismissCandidate = "dismiss_candidate"
)

// ProjectionMutation describes a mutation to a projection read model.
type ProjectionMutation struct {
	MutationType   string          `json:"mutation_type"`
	EntityID       string          `json:"entity_id"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// RecallMutation describes a mutation to a recall candidate.
type RecallMutation struct {
	MutationType   string          `json:"mutation_type"`
	EntityID       string          `json:"entity_id"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// CurationMutation describes a mutation to curation state (lens/dismiss/pin).
type CurationMutation struct {
	MutationType   string          `json:"mutation_type"`
	EntityID       string          `json:"entity_id"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// ProjectionMutator is the official write path for projection mutations.
type ProjectionMutator interface {
	ApplyProjectionMutation(ctx context.Context, mutation ProjectionMutation) error
}

// RecallMutator is the official write path for recall mutations.
type RecallMutator interface {
	ApplyRecallMutation(ctx context.Context, mutation RecallMutation) error
}

// CurationMutator is the official write path for curation state mutations.
type CurationMutator interface {
	ApplyCurationMutation(ctx context.Context, mutation CurationMutation) error
}

// BuildIdempotencyKey creates a deterministic key for deduplication.
func BuildIdempotencyKey(mutationType, entityID string) string {
	return fmt.Sprintf("%s:%s", mutationType, entityID)
}
