package usecase

import "context"

type generalStrategy struct {
	retrieve RetrieveContextUsecase
}

// NewGeneralStrategy wraps the existing RetrieveContextUsecase as a strategy.
func NewGeneralStrategy(retrieve RetrieveContextUsecase) RetrievalStrategy {
	return &generalStrategy{retrieve: retrieve}
}

func (s *generalStrategy) Name() string { return "general" }

func (s *generalStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, _ QueryIntent) (*RetrieveContextOutput, error) {
	return s.retrieve.Execute(ctx, input)
}
