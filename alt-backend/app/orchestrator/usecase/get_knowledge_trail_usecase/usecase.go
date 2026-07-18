package get_knowledge_trail_usecase

import (
	"context"

	"alt/domain"
	"alt/orchestrator/port/knowledge_trail_port"
	"alt/orchestrator/port/trail_thumbnail_port"
	"alt/orchestrator/usecase/trail_thumbnail_enrichment"

	"github.com/google/uuid"
)

const defaultLimit = 20

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
	episodes = trail_thumbnail_enrichment.Enrich(ctx, u.thumbnailPort, episodes)
	return &Result{Footprints: footprints, Branches: branches, Episodes: episodes, NextCursor: nextCursor, HasMore: hasMore}, nil
}
