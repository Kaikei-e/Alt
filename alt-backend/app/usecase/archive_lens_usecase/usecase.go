package archive_lens_usecase

import (
	"alt/port/knowledge_lens_port"
	"context"
	"fmt"

	"github.com/google/uuid"
)

type ArchiveLensUsecase struct {
	getLens    knowledge_lens_port.GetLensPort
	archivePort knowledge_lens_port.ArchiveLensPort
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

	return u.archivePort.ArchiveLens(ctx, lensID)
}
