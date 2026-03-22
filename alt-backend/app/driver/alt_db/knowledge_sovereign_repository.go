package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"fmt"
)

// ApplyProjectionMutation dispatches a projection mutation to the appropriate repository method.
func (r *AltDBRepository) ApplyProjectionMutation(ctx context.Context, mutation knowledge_sovereign_port.ProjectionMutation) error {
	switch mutation.MutationType {
	case knowledge_sovereign_port.MutationUpsertHomeItem:
		var item domain.KnowledgeHomeItem
		if err := json.Unmarshal(mutation.Payload, &item); err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		return r.UpsertKnowledgeHomeItem(ctx, item)

	case knowledge_sovereign_port.MutationDismissHomeItem:
		var params struct {
			UserID            string `json:"user_id"`
			ItemKey           string `json:"item_key"`
			ProjectionVersion int    `json:"projection_version"`
			DismissedAt       string `json:"dismissed_at"`
		}
		if err := json.Unmarshal(mutation.Payload, &params); err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		return nil // dismiss dispatch handled at curation level

	case knowledge_sovereign_port.MutationClearSupersede:
		return nil // clear supersede dispatch - detailed in Phase 2

	case knowledge_sovereign_port.MutationUpsertTodayDigest:
		var digest domain.TodayDigest
		if err := json.Unmarshal(mutation.Payload, &digest); err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		return r.UpsertTodayDigest(ctx, digest)

	case knowledge_sovereign_port.MutationUpsertRecallCandidate:
		var candidate domain.RecallCandidate
		if err := json.Unmarshal(mutation.Payload, &candidate); err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		return r.UpsertRecallCandidate(ctx, candidate)

	default:
		return fmt.Errorf("ApplyProjectionMutation: unknown projection mutation type: %s", mutation.MutationType)
	}
}

// ApplyRecallMutation dispatches a recall mutation to the appropriate repository method.
func (r *AltDBRepository) ApplyRecallMutation(ctx context.Context, mutation knowledge_sovereign_port.RecallMutation) error {
	switch mutation.MutationType {
	case knowledge_sovereign_port.MutationUpsertCandidate:
		var candidate domain.RecallCandidate
		if err := json.Unmarshal(mutation.Payload, &candidate); err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		return r.UpsertRecallCandidate(ctx, candidate)

	case knowledge_sovereign_port.MutationSnoozeCandidate:
		return nil // snooze dispatch - detailed params in Phase 2

	case knowledge_sovereign_port.MutationDismissCandidate:
		return nil // dismiss dispatch - detailed params in Phase 2

	default:
		return fmt.Errorf("ApplyRecallMutation: unknown recall mutation type: %s", mutation.MutationType)
	}
}

// ApplyCurationMutation dispatches a curation mutation to the appropriate repository method.
func (r *AltDBRepository) ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	switch mutation.MutationType {
	case knowledge_sovereign_port.MutationDismissCuration:
		return nil // dismiss curation dispatch - wired to DismissKnowledgeHomeItem in Phase 2

	default:
		return fmt.Errorf("ApplyCurationMutation: unknown curation mutation type: %s", mutation.MutationType)
	}
}

// ResolveRetentionDecision resolves retention using the policy matrix.
func (r *AltDBRepository) ResolveRetentionDecision(_ context.Context, entityType string, _ string) (domain.RetentionPolicy, error) {
	policy, ok := domain.DefaultRetentionMatrix[entityType]
	if !ok {
		return domain.RetentionPolicy{}, fmt.Errorf("ResolveRetentionDecision: unknown entity type: %s", entityType)
	}
	return policy, nil
}

// ResolveExportScope resolves export classification using the classification map.
func (r *AltDBRepository) ResolveExportScope(_ context.Context, entityType string, _ string) (domain.ExportClassification, error) {
	cls, ok := domain.DefaultExportClassification[entityType]
	if !ok {
		return domain.ExportClassification{}, fmt.Errorf("ResolveExportScope: unknown entity type: %s", entityType)
	}
	return cls, nil
}
