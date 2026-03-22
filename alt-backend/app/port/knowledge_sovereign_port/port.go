package knowledge_sovereign_port

import (
	"alt/domain"
	"context"
	"encoding/json"
)

// ProjectionMutation describes a mutation to a projection read model.
type ProjectionMutation struct {
	MutationType string          `json:"mutation_type"`
	EntityID     string          `json:"entity_id"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// RecallMutation describes a mutation to a recall candidate.
type RecallMutation struct {
	MutationType string          `json:"mutation_type"`
	EntityID     string          `json:"entity_id"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// CurationMutation describes a mutation to curation state (lens/dismiss/pin).
type CurationMutation struct {
	MutationType string          `json:"mutation_type"`
	EntityID     string          `json:"entity_id"`
	Payload      json.RawMessage `json:"payload,omitempty"`
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

// RetentionResolver resolves the retention decision for an entity.
type RetentionResolver interface {
	ResolveRetentionDecision(ctx context.Context, entityType string, entityID string) (domain.RetentionPolicy, error)
}

// ExportScopeResolver resolves the export scope for an entity.
type ExportScopeResolver interface {
	ResolveExportScope(ctx context.Context, entityType string, entityID string) (domain.ExportClassification, error)
}
