package reading_theme_port

import (
	"context"

	"github.com/google/uuid"

	"alt/domain"
)

type ListReadingThemesPort interface {
	ListReadingThemes(ctx context.Context, userID uuid.UUID, limit int32) ([]domain.ReadingThemeSummary, error)
}

type GetThemePort interface {
	GetTheme(ctx context.Context, userID uuid.UUID, themeID uuid.UUID, version *int32) (*domain.ReadingThemePage, error)
}
