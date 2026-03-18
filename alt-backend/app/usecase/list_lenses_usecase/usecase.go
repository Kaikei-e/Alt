package list_lenses_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"context"

	"github.com/google/uuid"
)

type ListLensesUsecase struct {
	listPort    knowledge_lens_port.ListLensesPort
	currentPort knowledge_lens_port.GetCurrentLensSelectionPort
}

type Result struct {
	Lenses       []domain.KnowledgeLens
	ActiveLensID *uuid.UUID
}

func NewListLensesUsecase(
	listPort knowledge_lens_port.ListLensesPort,
	currentPort knowledge_lens_port.GetCurrentLensSelectionPort,
) *ListLensesUsecase {
	return &ListLensesUsecase{listPort: listPort, currentPort: currentPort}
}

func (u *ListLensesUsecase) Execute(ctx context.Context, userID uuid.UUID) (*Result, error) {
	lenses, err := u.listPort.ListLenses(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := &Result{Lenses: lenses}
	if u.currentPort != nil {
		current, err := u.currentPort.GetCurrentLensSelection(ctx, userID)
		if err == nil && current != nil {
			result.ActiveLensID = &current.LensID
		}
	}
	return result, nil
}
