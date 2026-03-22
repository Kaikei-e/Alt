package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"errors"
)

// ErrKnowledgeSovereignNotImplemented is returned by stub methods
// that will be implemented in Phase 1+.
var ErrKnowledgeSovereignNotImplemented = errors.New("knowledge sovereign: not yet implemented")

// ApplyProjectionMutation is a stub for projection mutation writes.
func (r *AltDBRepository) ApplyProjectionMutation(_ context.Context, _ knowledge_sovereign_port.ProjectionMutation) error {
	return ErrKnowledgeSovereignNotImplemented
}

// ApplyRecallMutation is a stub for recall mutation writes.
func (r *AltDBRepository) ApplyRecallMutation(_ context.Context, _ knowledge_sovereign_port.RecallMutation) error {
	return ErrKnowledgeSovereignNotImplemented
}

// ApplyCurationMutation is a stub for curation state mutation writes.
func (r *AltDBRepository) ApplyCurationMutation(_ context.Context, _ knowledge_sovereign_port.CurationMutation) error {
	return ErrKnowledgeSovereignNotImplemented
}

// ResolveRetentionDecision is a stub for retention decision resolution.
func (r *AltDBRepository) ResolveRetentionDecision(_ context.Context, _ string, _ string) (domain.RetentionPolicy, error) {
	return domain.RetentionPolicy{}, ErrKnowledgeSovereignNotImplemented
}

// ResolveExportScope is a stub for export scope resolution.
func (r *AltDBRepository) ResolveExportScope(_ context.Context, _ string, _ string) (domain.ExportClassification, error) {
	return domain.ExportClassification{}, ErrKnowledgeSovereignNotImplemented
}
