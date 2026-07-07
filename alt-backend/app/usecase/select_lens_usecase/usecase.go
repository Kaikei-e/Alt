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

// NewSelectLensUsecase wires the four Knowledge Lens ports. All four are
// required — di/knowledge_module.go always passes the sovereign client for
// each — so a nil argument here is a composition-root wiring bug, not an
// intentionally-disabled feature. Rejecting nil at construction time (rather
// than the previous `if u.clearPort == nil { return nil }` deep inside
// Execute) surfaces that bug at startup instead of faking a successful lens
// clear (CLAUDE.md rule 8 / .claude/rules/di-wiring.md).
func NewSelectLensUsecase(
	getLens knowledge_lens_port.GetLensPort,
	getVersion knowledge_lens_port.GetCurrentLensVersionPort,
	selectPort knowledge_lens_port.SelectCurrentLensPort,
	clearPort knowledge_lens_port.ClearCurrentLensPort,
) *SelectLensUsecase {
	if getLens == nil || getVersion == nil || selectPort == nil || clearPort == nil {
		panic("select_lens_usecase: all four knowledge_lens_port dependencies are required and must be wired at composition root (see .claude/rules/di-wiring.md)")
	}
	return &SelectLensUsecase{
		getLens:    getLens,
		getVersion: getVersion,
		selectPort: selectPort,
		clearPort:  clearPort,
	}
}

func (u *SelectLensUsecase) Execute(ctx context.Context, userID uuid.UUID, lensID uuid.UUID) error {
	if lensID == uuid.Nil {
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
