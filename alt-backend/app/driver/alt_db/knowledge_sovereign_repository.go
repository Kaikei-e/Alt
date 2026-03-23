package alt_db

import (
	"alt/domain"
	"alt/port/knowledge_sovereign_port"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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
		userID, err := uuid.Parse(params.UserID)
		if err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): parse user_id: %w", mutation.MutationType, err)
		}
		dismissedAt, err := time.Parse(time.RFC3339Nano, params.DismissedAt)
		if err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): parse dismissed_at: %w", mutation.MutationType, err)
		}
		return r.DismissKnowledgeHomeItem(ctx, userID, params.ItemKey, params.ProjectionVersion, dismissedAt)

	case knowledge_sovereign_port.MutationClearSupersede:
		var params struct {
			UserID            string `json:"user_id"`
			ItemKey           string `json:"item_key"`
			ProjectionVersion int    `json:"projection_version"`
		}
		if err := json.Unmarshal(mutation.Payload, &params); err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		userID, err := uuid.Parse(params.UserID)
		if err != nil {
			return fmt.Errorf("ApplyProjectionMutation(%s): parse user_id: %w", mutation.MutationType, err)
		}
		return r.ClearSupersedeState(ctx, userID, params.ItemKey, params.ProjectionVersion)

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
		var params struct {
			UserID  string `json:"user_id"`
			ItemKey string `json:"item_key"`
			Until   string `json:"until"`
		}
		if err := json.Unmarshal(mutation.Payload, &params); err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		userID, err := uuid.Parse(params.UserID)
		if err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): parse user_id: %w", mutation.MutationType, err)
		}
		until, err := time.Parse(time.RFC3339Nano, params.Until)
		if err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): parse until: %w", mutation.MutationType, err)
		}
		return r.SnoozeRecallCandidate(ctx, userID, params.ItemKey, until)

	case knowledge_sovereign_port.MutationDismissCandidate:
		var params struct {
			UserID  string `json:"user_id"`
			ItemKey string `json:"item_key"`
		}
		if err := json.Unmarshal(mutation.Payload, &params); err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		userID, err := uuid.Parse(params.UserID)
		if err != nil {
			return fmt.Errorf("ApplyRecallMutation(%s): parse user_id: %w", mutation.MutationType, err)
		}
		return r.DismissRecallCandidate(ctx, userID, params.ItemKey)

	default:
		return fmt.Errorf("ApplyRecallMutation: unknown recall mutation type: %s", mutation.MutationType)
	}
}

// ApplyCurationMutation dispatches a curation mutation to the appropriate repository method.
func (r *AltDBRepository) ApplyCurationMutation(ctx context.Context, mutation knowledge_sovereign_port.CurationMutation) error {
	switch mutation.MutationType {
	case knowledge_sovereign_port.MutationDismissCuration:
		var params struct {
			UserID            string `json:"user_id"`
			ItemKey           string `json:"item_key"`
			ProjectionVersion int    `json:"projection_version"`
			DismissedAt       string `json:"dismissed_at"`
		}
		if err := json.Unmarshal(mutation.Payload, &params); err != nil {
			return fmt.Errorf("ApplyCurationMutation(%s): unmarshal: %w", mutation.MutationType, err)
		}
		userID, err := uuid.Parse(params.UserID)
		if err != nil {
			return fmt.Errorf("ApplyCurationMutation(%s): parse user_id: %w", mutation.MutationType, err)
		}
		// projection_version と dismissed_at はオプショナル。
		// TrackHomeActionUsecase は user_id と item_key のみ送る。
		projectionVersion := params.ProjectionVersion
		if projectionVersion == 0 {
			projectionVersion = 1
		}
		dismissedAt := time.Now()
		if params.DismissedAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, params.DismissedAt)
			if err != nil {
				return fmt.Errorf("ApplyCurationMutation(%s): parse dismissed_at: %w", mutation.MutationType, err)
			}
			dismissedAt = parsed
		}
		return r.DismissKnowledgeHomeItem(ctx, userID, params.ItemKey, projectionVersion, dismissedAt)

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
