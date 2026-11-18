package sovereign_client

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

type ReadingThemeRow struct {
	ThemeID           uuid.UUID
	Title             string
	LatestVersion     int32
	ArticleCount      int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastOpenedAt      *time.Time
	LastOpenedVersion int32
}

func (c *Client) ListReadingThemes(ctx context.Context, userID uuid.UUID, limit int32) ([]ReadingThemeRow, error) {
	if !c.enabled {
		return nil, ErrSovereignDisabled
	}

	resp, err := c.client.ListReadingThemes(ctx, connect.NewRequest(&sovereignv1.ListReadingThemesRequest{
		UserId: userID.String(),
		Limit:  limit,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListReadingThemes: %w", err)
	}

	rows := make([]ReadingThemeRow, 0, len(resp.Msg.GetThemes()))
	for _, pb := range resp.Msg.GetThemes() {
		themeID, err := uuid.Parse(pb.GetThemeId())
		if err != nil {
			return nil, fmt.Errorf("sovereign ListReadingThemes invalid theme_id %q: %w", pb.GetThemeId(), err)
		}
		var createdAt, updatedAt time.Time
		if pb.GetCreatedAt() != nil {
			createdAt = pb.GetCreatedAt().AsTime()
		}
		if pb.GetUpdatedAt() != nil {
			updatedAt = pb.GetUpdatedAt().AsTime()
		}
		var lastOpenedAt *time.Time
		if pb.GetLastOpenedAt() != nil {
			t := pb.GetLastOpenedAt().AsTime()
			lastOpenedAt = &t
		}
		rows = append(rows, ReadingThemeRow{
			ThemeID:           themeID,
			Title:             pb.GetTitle(),
			LatestVersion:     pb.GetLatestVersion(),
			ArticleCount:      pb.GetArticleCount(),
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
			LastOpenedAt:      lastOpenedAt,
			LastOpenedVersion: pb.GetLastOpenedVersion(),
		})
	}
	return rows, nil
}
