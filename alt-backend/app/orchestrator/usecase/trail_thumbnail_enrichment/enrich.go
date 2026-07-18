// Package trail_thumbnail_enrichment resolves each episode's representative
// (first/newest member) OG image (D29). Shared by every usecase that returns
// a TrailEpisode page (get_knowledge_trail_usecase, search_trail_usecase) so
// thumbnail resolution rules never diverge between spine reads.
package trail_thumbnail_enrichment

import (
	"context"
	"log/slog"
	"strings"

	"alt/domain"
	"alt/orchestrator/port/trail_thumbnail_port"

	"github.com/google/uuid"
)

// articleItemKeyPrefix marks an item_key that anchors back to an `articles`
// row (item_key = "article:<uuid>"). Only article representatives are
// eligible for OG-image thumbnail enrichment (D29).
const articleItemKeyPrefix = "article:"

// Enrich resolves each episode's representative (first/newest member) OG
// image (D29). A lookup miss, a non-article representative, or a lookup
// failure all degrade to an empty ThumbnailURL rather than failing the
// caller — the frontend falls back to a text-only card.
func Enrich(ctx context.Context, port trail_thumbnail_port.GetOgImageURLsPort, episodes []domain.TrailEpisode) []domain.TrailEpisode {
	articleIDByEpisode := make(map[int]string, len(episodes))
	var ids []string
	for i, ep := range episodes {
		if len(ep.Footprints) == 0 {
			continue
		}
		articleID, ok := articleIDFromItemKey(ep.Footprints[0].ItemKey)
		if !ok {
			continue
		}
		articleIDByEpisode[i] = articleID
		ids = append(ids, articleID)
	}
	if len(ids) == 0 {
		return episodes
	}

	urls, err := port.GetOgImageURLsByArticleIDs(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "trail episode thumbnail lookup failed, degrading to text", "error", err)
		return episodes
	}

	for i, articleID := range articleIDByEpisode {
		episodes[i].ThumbnailURL = urls[articleID]
	}
	return episodes
}

// articleIDFromItemKey extracts the article id from an "article:<uuid>"
// item_key. A malformed or non-article item_key is not eligible for
// thumbnail lookup.
func articleIDFromItemKey(itemKey string) (string, bool) {
	id, ok := strings.CutPrefix(itemKey, articleItemKeyPrefix)
	if !ok || id == "" {
		return "", false
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", false
	}
	return id, true
}
