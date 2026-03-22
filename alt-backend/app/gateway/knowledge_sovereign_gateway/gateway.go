package knowledge_sovereign_gateway

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"fmt"
)

// KnowledgeSovereignRepo defines the driver methods used by this gateway.
type KnowledgeSovereignRepo interface {
	ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error
	ApplyRecallMutation(ctx context.Context, mutation knowledge_sovereign_port.RecallMutation) error
	ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error
	ResolveRetentionDecision(ctx context.Context, entityType string, entityID string) (domain.RetentionPolicy, error)
	ResolveExportScope(ctx context.Context, entityType string, entityID string) (domain.ExportClassification, error)
}

// Gateway implements knowledge sovereign port interfaces.
type Gateway struct {
	repo KnowledgeSovereignRepo
}

// NewGateway creates a new knowledge sovereign gateway.
func NewGateway(repo KnowledgeSovereignRepo) *Gateway {
	return &Gateway{repo: repo}
}

// ApplyProjectionMutation implements knowledge_sovereign_port.ProjectionMutator.
func (g *Gateway) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if g.repo == nil {
		return fmt.Errorf("ApplyProjectionMutation: database connection not available")
	}
	return g.repo.ApplyProjectionMutation(ctx, mutation)
}

// ApplyRecallMutation implements knowledge_sovereign_port.RecallMutator.
func (g *Gateway) ApplyRecallMutation(ctx context.Context, mutation knowledge_sovereign_port.RecallMutation) error {
	if g.repo == nil {
		return fmt.Errorf("ApplyRecallMutation: database connection not available")
	}
	return g.repo.ApplyRecallMutation(ctx, mutation)
}

// ApplyCurationMutation implements knowledge_sovereign_port.CurationMutator.
func (g *Gateway) ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	if g.repo == nil {
		return fmt.Errorf("ApplyCurationMutation: database connection not available")
	}
	return g.repo.ApplyCurationMutation(ctx, mutation)
}

// ResolveRetentionDecision implements knowledge_sovereign_port.RetentionResolver.
func (g *Gateway) ResolveRetentionDecision(ctx context.Context, entityType string, entityID string) (domain.RetentionPolicy, error) {
	if g.repo == nil {
		return domain.RetentionPolicy{}, fmt.Errorf("ResolveRetentionDecision: database connection not available")
	}
	return g.repo.ResolveRetentionDecision(ctx, entityType, entityID)
}

// ResolveExportScope implements knowledge_sovereign_port.ExportScopeResolver.
func (g *Gateway) ResolveExportScope(ctx context.Context, entityType string, entityID string) (domain.ExportClassification, error) {
	if g.repo == nil {
		return domain.ExportClassification{}, fmt.Errorf("ResolveExportScope: database connection not available")
	}
	return g.repo.ResolveExportScope(ctx, entityType, entityID)
}
