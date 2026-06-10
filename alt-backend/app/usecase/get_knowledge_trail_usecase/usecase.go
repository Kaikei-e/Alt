package get_knowledge_trail_usecase

import (
	"context"

	"alt/domain"
	"alt/port/knowledge_trail_port"

	"github.com/google/uuid"
)

const defaultLimit = 20

// Result is the trail spine page returned to the handler.
type Result struct {
	Footprints []domain.TrailFootprint
	NextCursor string
	HasMore    bool
}

// GetKnowledgeTrailUsecase reads the user's footprint spine.
type GetKnowledgeTrailUsecase struct {
	trailPort knowledge_trail_port.GetTrailPort
}

func NewGetKnowledgeTrailUsecase(trailPort knowledge_trail_port.GetTrailPort) *GetKnowledgeTrailUsecase {
	return &GetKnowledgeTrailUsecase{trailPort: trailPort}
}

// Execute returns one page of the user's footprint spine, optionally filtered
// to a theme lens (filterTags).
func (u *GetKnowledgeTrailUsecase) Execute(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) (*Result, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultLimit
	}
	footprints, nextCursor, hasMore, err := u.trailPort.GetTrailFootprints(ctx, userID, cursor, limit, filterTags)
	if err != nil {
		return nil, err
	}
	return &Result{Footprints: footprints, NextCursor: nextCursor, HasMore: hasMore}, nil
}
