package reading_theme_usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_theme_port"
)

type GetThemeUsecase struct {
	getPort reading_theme_port.GetThemePort
}

func NewGetThemeUsecase(getPort reading_theme_port.GetThemePort) *GetThemeUsecase {
	if getPort == nil {
		panic("reading_theme_usecase: getPort is required")
	}
	return &GetThemeUsecase{getPort: getPort}
}

func (u *GetThemeUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	rawThemeID string,
	version *int32,
) (*domain.ReadingThemePage, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", domain.ErrInvalidThemeInput)
	}
	themeID, err := uuid.Parse(rawThemeID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid theme_id", domain.ErrInvalidThemeInput)
	}
	if version != nil && *version < 1 {
		return nil, fmt.Errorf("%w: version must be at least 1", domain.ErrInvalidThemeInput)
	}

	page, err := u.getPort.GetTheme(ctx, userID, themeID, version)
	if err != nil {
		return nil, fmt.Errorf("get theme: %w", err)
	}
	return page, nil
}
