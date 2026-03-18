package update_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type UpdateLensUsecase struct {
	getLens     knowledge_lens_port.GetLensPort
	versionPort knowledge_lens_port.CreateLensVersionPort
}

func NewUpdateLensUsecase(
	getLens knowledge_lens_port.GetLensPort,
	versionPort knowledge_lens_port.CreateLensVersionPort,
) *UpdateLensUsecase {
	return &UpdateLensUsecase{
		getLens:     getLens,
		versionPort: versionPort,
	}
}

type UpdateLensInput struct {
	LensID       uuid.UUID
	UserID       uuid.UUID
	Name         string
	Description  string
	QueryText    string
	TagIDs       []string
	FeedIDs      []string
	TimeWindow   string
	IncludeRecap bool
	IncludePulse bool
	SortMode     string
}

func (u *UpdateLensUsecase) Execute(ctx context.Context, input UpdateLensInput) (*domain.KnowledgeLensVersion, error) {
	lens, err := u.getLens.GetLens(ctx, input.LensID)
	if err != nil {
		return nil, fmt.Errorf("get lens: %w", err)
	}
	if lens.UserID != input.UserID {
		return nil, fmt.Errorf("lens does not belong to user")
	}

	if input.SortMode == "" {
		input.SortMode = "relevance"
	}

	version := domain.KnowledgeLensVersion{
		LensVersionID: uuid.New(),
		LensID:        input.LensID,
		CreatedAt:     time.Now(),
		QueryText:     input.QueryText,
		TagIDs:        input.TagIDs,
		FeedIDs:       input.FeedIDs,
		TimeWindow:    input.TimeWindow,
		IncludeRecap:  input.IncludeRecap,
		IncludePulse:  input.IncludePulse,
		SortMode:      input.SortMode,
	}

	if err := u.versionPort.CreateLensVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("create lens version: %w", err)
	}

	return &version, nil
}
