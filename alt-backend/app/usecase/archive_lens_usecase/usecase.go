package archive_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"alt/port/knowledge_sovereign_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type ArchiveLensUsecase struct {
	getLens         knowledge_lens_port.GetLensPort
	archivePort     knowledge_lens_port.ArchiveLensPort
	curationMutator knowledge_sovereign_port.CurationMutator
}

// SetCurationMutator wires the optional Knowledge Sovereign curation mutator.
func (u *ArchiveLensUsecase) SetCurationMutator(port knowledge_sovereign_port.CurationMutator) {
	u.curationMutator = port
}

func NewArchiveLensUsecase(
	getLens knowledge_lens_port.GetLensPort,
	archivePort knowledge_lens_port.ArchiveLensPort,
) *ArchiveLensUsecase {
	return &ArchiveLensUsecase{
		getLens:     getLens,
		archivePort: archivePort,
	}
}

func (u *ArchiveLensUsecase) Execute(ctx context.Context, userID uuid.UUID, lensID uuid.UUID) error {
	lens, err := u.getLens.GetLens(ctx, lensID)
	if err != nil {
		return fmt.Errorf("get lens: %w", err)
	}
	if lens.UserID != userID {
		return fmt.Errorf("lens does not belong to user")
	}

	if err := u.archivePort.ArchiveLens(ctx, lensID); err != nil {
		return fmt.Errorf("archive lens: %w", err)
	}

	if u.curationMutator != nil {
		archivePayload, _ := json.Marshal(map[string]any{
			"lens_id": lensID.String(),
			"user_id": userID.String(),
		})
		if err := u.curationMutator.ApplyCurationMutation(ctx, knowledge_sovereign_port.CurationMutation{
			MutationType:   knowledge_sovereign_port.MutationArchiveLens,
			EntityID:       lensID.String(),
			Payload:        archivePayload,
			IdempotencyKey: domain.BuildIdempotencyKey(knowledge_sovereign_port.MutationArchiveLens, lensID.String()),
		}); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to route lens archive through sovereign", "error", err)
		}
	}

	return nil
}
