package create_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CreateLensUsecase struct {
	lensPort    knowledge_lens_port.CreateLensPort
	versionPort knowledge_lens_port.CreateLensVersionPort
}

func NewCreateLensUsecase(
	lensPort knowledge_lens_port.CreateLensPort,
	versionPort knowledge_lens_port.CreateLensVersionPort,
) *CreateLensUsecase {
	return &CreateLensUsecase{
		lensPort:    lensPort,
		versionPort: versionPort,
	}
}

type CreateLensInput struct {
	UserID      uuid.UUID
	TenantID    uuid.UUID
	Name        string
	Description string
	QueryText   string
	TagIDs      []string
	TimeWindow  string
	IncludeRecap bool
	IncludePulse bool
	SortMode    string
}

type CreateLensResult struct {
	Lens    domain.KnowledgeLens
	Version domain.KnowledgeLensVersion
}

func (u *CreateLensUsecase) Execute(ctx context.Context, input CreateLensInput) (*CreateLensResult, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("lens name is required")
	}
	if input.SortMode == "" {
		input.SortMode = "relevance"
	}

	now := time.Now()
	lensID := uuid.New()
	versionID := uuid.New()

	lens := domain.KnowledgeLens{
		LensID:    lensID,
		UserID:    input.UserID,
		TenantID:  input.TenantID,
		Name:      input.Name,
		Description: input.Description,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := u.lensPort.CreateLens(ctx, lens); err != nil {
		return nil, fmt.Errorf("create lens: %w", err)
	}

	version := domain.KnowledgeLensVersion{
		LensVersionID: versionID,
		LensID:        lensID,
		CreatedAt:     now,
		QueryText:     input.QueryText,
		TagIDs:        input.TagIDs,
		TimeWindow:    input.TimeWindow,
		IncludeRecap:  input.IncludeRecap,
		IncludePulse:  input.IncludePulse,
		SortMode:      input.SortMode,
	}

	if err := u.versionPort.CreateLensVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("create lens version: %w", err)
	}

	lens.CurrentVersion = &version
	return &CreateLensResult{Lens: lens, Version: version}, nil
}
