package get_knowledge_trail_usecase

import (
	"context"
	"log/slog"
	"strings"

	"alt/domain"
	"alt/orchestrator/port/knowledge_trail_port"
	"alt/orchestrator/port/trail_thumbnail_port"

	"github.com/google/uuid"
)

const defaultLimit = 20

// articleItemKeyPrefix marks an item_key that anchors back to an `articles`
// row (item_key = "article:<uuid>"). Only article representatives are
// eligible for OG-image thumbnail enrichment (D29).
const articleItemKeyPrefix = "article:"

// Result is the trail spine page returned to the handler.
type Result struct {
	Footprints []domain.TrailFootprint
	Branches   []domain.TrailBranch
	// Episodes are the spine's default display unit (D24/D30, Wave 8).
	Episodes   []domain.TrailEpisode
	NextCursor string
	HasMore    bool
}

// GetKnowledgeTrailUsecase reads the user's footprint spine.
type GetKnowledgeTrailUsecase struct {
	trailPort     knowledge_trail_port.GetTrailPort
	thumbnailPort trail_thumbnail_port.GetOgImageURLsPort
}

func NewGetKnowledgeTrailUsecase(trailPort knowledge_trail_port.GetTrailPort, thumbnailPort trail_thumbnail_port.GetOgImageURLsPort) *GetKnowledgeTrailUsecase {
	return &GetKnowledgeTrailUsecase{trailPort: trailPort, thumbnailPort: thumbnailPort}
}

// Execute returns one page of the user's footprint spine, optionally filtered
// to a theme lens (filterTags).
func (u *GetKnowledgeTrailUsecase) Execute(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) (*Result, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultLimit
	}
	footprints, branches, episodes, nextCursor, hasMore, err := u.trailPort.GetTrailFootprints(ctx, userID, cursor, limit, filterTags)
	if err != nil {
		return nil, err
	}
	episodes = u.enrichThumbnails(ctx, episodes)
	return &Result{Footprints: footprints, Branches: branches, Episodes: episodes, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// enrichThumbnails resolves each episode's representative (first/newest
// member) OG image (D29). A lookup miss, a non-article representative, or a
// lookup failure all degrade to an empty ThumbnailURL rather than failing
// the trail read — the frontend falls back to a text-only card.
func (u *GetKnowledgeTrailUsecase) enrichThumbnails(ctx context.Context, episodes []domain.TrailEpisode) []domain.TrailEpisode {
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

	urls, err := u.thumbnailPort.GetOgImageURLsByArticleIDs(ctx, ids)
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
