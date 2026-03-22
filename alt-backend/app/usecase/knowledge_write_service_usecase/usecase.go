package knowledge_write_service_usecase

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"fmt"
)

// KnowledgeWriteServiceUsecase is the single entry point for all
// knowledge state write operations. Future write paths will be
// consolidated here.
type KnowledgeWriteServiceUsecase struct {
	projectionMutator knowledge_sovereign_port.ProjectionMutator
	recallMutator     knowledge_sovereign_port.RecallMutator
	curationMutator   knowledge_sovereign_port.CurationMutator
	retentionResolver knowledge_sovereign_port.RetentionResolver
	exportResolver    knowledge_sovereign_port.ExportScopeResolver
}

// NewKnowledgeWriteServiceUsecase creates a new KnowledgeWriteServiceUsecase.
func NewKnowledgeWriteServiceUsecase(
	projectionMutator knowledge_sovereign_port.ProjectionMutator,
	recallMutator knowledge_sovereign_port.RecallMutator,
	curationMutator knowledge_sovereign_port.CurationMutator,
	retentionResolver knowledge_sovereign_port.RetentionResolver,
	exportResolver knowledge_sovereign_port.ExportScopeResolver,
) *KnowledgeWriteServiceUsecase {
	return &KnowledgeWriteServiceUsecase{
		projectionMutator: projectionMutator,
		recallMutator:     recallMutator,
		curationMutator:   curationMutator,
		retentionResolver: retentionResolver,
		exportResolver:    exportResolver,
	}
}

// ApplyProjectionMutation delegates projection mutations to the official write path.
func (u *KnowledgeWriteServiceUsecase) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	if err := u.projectionMutator.ApplyProjectionMutation(ctx, mutation); err != nil {
		return fmt.Errorf("apply projection mutation: %w", err)
	}
	return nil
}

// ApplyRecallMutation delegates recall mutations to the official write path.
func (u *KnowledgeWriteServiceUsecase) ApplyRecallMutation(ctx context.Context, mutation knowledge_sovereign_port.RecallMutation) error {
	if err := u.recallMutator.ApplyRecallMutation(ctx, mutation); err != nil {
		return fmt.Errorf("apply recall mutation: %w", err)
	}
	return nil
}

// ApplyCurationMutation delegates curation mutations to the official write path.
func (u *KnowledgeWriteServiceUsecase) ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	if err := u.curationMutator.ApplyCurationMutation(ctx, mutation); err != nil {
		return fmt.Errorf("apply curation mutation: %w", err)
	}
	return nil
}

// ResolveRetentionDecision delegates retention decisions to the official write path.
func (u *KnowledgeWriteServiceUsecase) ResolveRetentionDecision(ctx context.Context, entityType string, entityID string) (domain.RetentionPolicy, error) {
	policy, err := u.retentionResolver.ResolveRetentionDecision(ctx, entityType, entityID)
	if err != nil {
		return domain.RetentionPolicy{}, fmt.Errorf("resolve retention decision: %w", err)
	}
	return policy, nil
}

// ResolveExportScope delegates export scope resolution to the official write path.
func (u *KnowledgeWriteServiceUsecase) ResolveExportScope(ctx context.Context, entityType string, entityID string) (domain.ExportClassification, error) {
	classification, err := u.exportResolver.ResolveExportScope(ctx, entityType, entityID)
	if err != nil {
		return domain.ExportClassification{}, fmt.Errorf("resolve export scope: %w", err)
	}
	return classification, nil
}
