package list_lenses_usecase

import (
	"alt/domain"
	"alt/port/knowledge_lens_port"
	"context"

	"github.com/google/uuid"
)

type ListLensesUsecase struct {
	listPort knowledge_lens_port.ListLensesPort
}

func NewListLensesUsecase(listPort knowledge_lens_port.ListLensesPort) *ListLensesUsecase {
	return &ListLensesUsecase{listPort: listPort}
}

func (u *ListLensesUsecase) Execute(ctx context.Context, userID uuid.UUID) ([]domain.KnowledgeLens, error) {
	return u.listPort.ListLenses(ctx, userID)
}
