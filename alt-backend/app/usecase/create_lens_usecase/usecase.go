package create_lens_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"alt/port/knowledge_sovereign_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CreateLensUsecase struct {
	lensPort        knowledge_lens_port.CreateLensPort
	versionPort     knowledge_lens_port.CreateLensVersionPort
	curationMutator knowledge_sovereign_port.CurationMutator
}

// SetCurationMutator wires the optional Knowledge Sovereign curation mutator.
func (u *CreateLensUsecase) SetCurationMutator(port knowledge_sovereign_port.CurationMutator) {
	u.curationMutator = port
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
	UserID       uuid.UUID
	TenantID     uuid.UUID
	Name         string
	Description  string
	QueryText    string
	TagIDs       []string
	SourceIDs    []string
	TimeWindow   string
	IncludeRecap bool
	IncludePulse bool
	SortMode     string
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
		LensID:      lensID,
		UserID:      input.UserID,
		TenantID:    input.TenantID,
		Name:        input.Name,
		Description: input.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
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
		SourceIDs:     input.SourceIDs,
		TimeWindow:    input.TimeWindow,
		IncludeRecap:  input.IncludeRecap,
		IncludePulse:  input.IncludePulse,
		SortMode:      input.SortMode,
	}

	if err := u.versionPort.CreateLensVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("create lens version: %w", err)
	}

	lens.CurrentVersion = &version

	if u.curationMutator != nil {
		lensPayload, _ := json.Marshal(map[string]any{
			"lens_id":   lensID.String(),
			"user_id":   input.UserID.String(),
			"tenant_id": input.TenantID.String(),
			"name":      input.Name,
		})
		if err := u.curationMutator.ApplyCurationMutation(ctx, knowledge_sovereign_port.CurationMutation{
			MutationType:   knowledge_sovereign_port.MutationCreateLens,
			EntityID:       lensID.String(),
			Payload:        lensPayload,
			IdempotencyKey: domain.BuildIdempotencyKey(knowledge_sovereign_port.MutationCreateLens, lensID.String()),
		}); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to route lens creation through sovereign", "error", err)
		}

		versionPayload, _ := json.Marshal(map[string]any{
			"lens_version_id": versionID.String(),
			"lens_id":         lensID.String(),
		})
		if err := u.curationMutator.ApplyCurationMutation(ctx, knowledge_sovereign_port.CurationMutation{
			MutationType:   knowledge_sovereign_port.MutationCreateLensVersion,
			EntityID:       versionID.String(),
			Payload:        versionPayload,
			IdempotencyKey: domain.BuildIdempotencyKey(knowledge_sovereign_port.MutationCreateLensVersion, versionID.String()),
		}); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to route lens version creation through sovereign", "error", err)
		}
	}

	return &CreateLensResult{Lens: lens, Version: version}, nil
}
