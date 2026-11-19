package reading_theme_usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_theme_port"
)

type ListThemesUsecase struct {
	listPort reading_theme_port.ListReadingThemesPort
}

func NewListThemesUsecase(listPort reading_theme_port.ListReadingThemesPort) *ListThemesUsecase {
	if listPort == nil {
		panic("reading_theme_usecase: listPort is required")
	}
	return &ListThemesUsecase{listPort: listPort}
}

func (u *ListThemesUsecase) Execute(ctx context.Context, userID uuid.UUID) ([]domain.ReadingThemeSummary, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidThemeInput)
	}
	themes, err := u.listPort.ListReadingThemes(ctx, userID, 0)
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	return themes, nil
}
