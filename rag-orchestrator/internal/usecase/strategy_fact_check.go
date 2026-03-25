package usecase

import (
	"context"
	"log/slog"
)

type factCheckStrategy struct {
	retrieve RetrieveContextUsecase
	logger   *slog.Logger
}

// NewFactCheckStrategy creates a strategy for fact-check queries.
// It uses the general pipeline — prompt builder will structure the response
// as claim → evidence → judgment.
func NewFactCheckStrategy(retrieve RetrieveContextUsecase, logger *slog.Logger) RetrievalStrategy {
	return &factCheckStrategy{retrieve: retrieve, logger: logger}
}

func (s *factCheckStrategy) Name() string { return "fact_check" }

func (s *factCheckStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	s.logger.Info("fact_check_strategy_execute",
		slog.String("query", input.Query))

	return s.retrieve.Execute(ctx, input)
}
