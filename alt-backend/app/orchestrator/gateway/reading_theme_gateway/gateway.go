package reading_theme_gateway

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/reading_theme_port"
	"alt/shared/driver/sovereign_client"
)

type listReadingThemesDriver interface {
	ListReadingThemes(ctx context.Context, userID uuid.UUID, limit int32) ([]sovereign_client.ReadingThemeRow, error)
}

type Gateway struct {
	driver listReadingThemesDriver
}

var _ reading_theme_port.ListReadingThemesPort = (*Gateway)(nil)

func NewGateway(driver listReadingThemesDriver) *Gateway {
	if driver == nil {
		panic("reading_theme_gateway: driver is required")
	}
	return &Gateway{driver: driver}
}

func (g *Gateway) ListReadingThemes(ctx context.Context, userID uuid.UUID, limit int32) ([]domain.ReadingThemeSummary, error) {
	rows, err := g.driver.ListReadingThemes(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list reading themes: %w", err)
	}

	out := make([]domain.ReadingThemeSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ReadingThemeSummary{
			ThemeID:           r.ThemeID,
			Title:             r.Title,
			LatestVersion:     r.LatestVersion,
			ArticleCount:      r.ArticleCount,
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
			LastOpenedAt:      r.LastOpenedAt,
			LastOpenedVersion: r.LastOpenedVersion,
		})
	}
	return out, nil
}
