package select_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"alt/port/knowledge_sovereign_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SelectLensUsecase struct {
	getLens         knowledge_lens_port.GetLensPort
	getVersion      knowledge_lens_port.GetCurrentLensVersionPort
	selectPort      knowledge_lens_port.SelectCurrentLensPort
	clearPort       knowledge_lens_port.ClearCurrentLensPort
	curationMutator knowledge_sovereign_port.CurationMutator
}

// SetCurationMutator wires the optional Knowledge Sovereign curation mutator.
func (u *SelectLensUsecase) SetCurationMutator(port knowledge_sovereign_port.CurationMutator) {
	u.curationMutator = port
}

func NewSelectLensUsecase(
	getLens knowledge_lens_port.GetLensPort,
	getVersion knowledge_lens_port.GetCurrentLensVersionPort,
	selectPort knowledge_lens_port.SelectCurrentLensPort,
	clearPort knowledge_lens_port.ClearCurrentLensPort,
) *SelectLensUsecase {
	return &SelectLensUsecase{
		getLens:    getLens,
		getVersion: getVersion,
		selectPort: selectPort,
		clearPort:  clearPort,
	}
}

func (u *SelectLensUsecase) Execute(ctx context.Context, userID uuid.UUID, lensID uuid.UUID) error {
	if lensID == uuid.Nil {
		if u.clearPort == nil {
			return nil
		}
		if err := u.clearPort.ClearCurrentLens(ctx, userID); err != nil {
			return err
		}

		if u.curationMutator != nil {
			clearPayload, _ := json.Marshal(map[string]any{
				"user_id": userID.String(),
			})
			if err := u.curationMutator.ApplyCurationMutation(ctx, knowledge_sovereign_port.CurationMutation{
				MutationType:   knowledge_sovereign_port.MutationClearLens,
				EntityID:       userID.String(),
				Payload:        clearPayload,
				IdempotencyKey: domain.BuildIdempotencyKey(knowledge_sovereign_port.MutationClearLens, userID.String()),
			}); err != nil {
				logger.Logger.ErrorContext(ctx, "failed to route lens clear through sovereign", "error", err)
			}
		}

		return nil
	}
	lens, err := u.getLens.GetLens(ctx, lensID)
	if err != nil {
		return fmt.Errorf("get lens: %w", err)
	}
	if lens.UserID != userID {
		return fmt.Errorf("lens does not belong to user")
	}

	version, err := u.getVersion.GetCurrentLensVersion(ctx, lensID)
	if err != nil {
		return fmt.Errorf("get current lens version: %w", err)
	}

	current := domain.KnowledgeCurrentLens{
		UserID:        userID,
		LensID:        lensID,
		LensVersionID: version.LensVersionID,
		SelectedAt:    time.Now(),
	}

	if err := u.selectPort.SelectCurrentLens(ctx, current); err != nil {
		return err
	}

	if u.curationMutator != nil {
		selectPayload, _ := json.Marshal(map[string]any{
			"user_id":         userID.String(),
			"lens_id":         lensID.String(),
			"lens_version_id": version.LensVersionID.String(),
		})
		if err := u.curationMutator.ApplyCurationMutation(ctx, knowledge_sovereign_port.CurationMutation{
			MutationType:   knowledge_sovereign_port.MutationSelectLens,
			EntityID:       lensID.String(),
			Payload:        selectPayload,
			IdempotencyKey: domain.BuildIdempotencyKey(knowledge_sovereign_port.MutationSelectLens, lensID.String()),
		}); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to route lens select through sovereign", "error", err)
		}
	}

	return nil
}
