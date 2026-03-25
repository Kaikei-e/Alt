package select_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SelectLensUsecase struct {
	getLens    knowledge_lens_port.GetLensPort
	getVersion knowledge_lens_port.GetCurrentLensVersionPort
	selectPort knowledge_lens_port.SelectCurrentLensPort
	clearPort  knowledge_lens_port.ClearCurrentLensPort
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
	if version == nil {
		return fmt.Errorf("no active version found for lens %s", lensID)
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

	return nil
}
