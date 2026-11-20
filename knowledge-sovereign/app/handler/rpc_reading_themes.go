package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

const (
	defaultReadingThemesLimit = 100
	maxReadingThemesLimit     = 500
)

// ListReadingThemes returns reading themes for the calling user ordered by
// updated_at descending and theme_id ascending.
func (h *SovereignHandler) ListReadingThemes(
	ctx context.Context,
	req *connect.Request[sovereignv1.ListReadingThemesRequest],
) (*connect.Response[sovereignv1.ListReadingThemesResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	limit := int(msg.GetLimit())
	if limit <= 0 {
		limit = defaultReadingThemesLimit
	} else if limit > maxReadingThemesLimit {
		limit = maxReadingThemesLimit
	}

	themes, err := h.readDB.ListReadingThemes(ctx, userID, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("ListReadingThemes: %w", err))
	}

	entries := make([]*sovereignv1.ReadingThemeEntry, 0, len(themes))
	for _, th := range themes {
		entry := &sovereignv1.ReadingThemeEntry{
			ThemeId:           th.ThemeID.String(),
			Title:             th.Title,
			LatestVersion:     safeIntToInt32(th.LatestVersion),
			ArticleCount:      safeIntToInt32(th.ArticleCount),
			CreatedAt:         timestamppb.New(th.CreatedAt),
			UpdatedAt:         timestamppb.New(th.UpdatedAt),
			LastOpenedVersion: safeIntToInt32(th.LastOpenedVersion),
		}
		if th.LastOpenedAt != nil {
			entry.LastOpenedAt = timestamppb.New(*th.LastOpenedAt)
		}
		entries = append(entries, entry)
	}

	return connect.NewResponse(&sovereignv1.ListReadingThemesResponse{
		Themes: entries,
	}), nil
}
