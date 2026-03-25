package usecase

import (
	"context"
	"log/slog"
)

type topicDeepDiveStrategy struct {
	retrieve RetrieveContextUsecase
	logger   *slog.Logger
}

// NewTopicDeepDiveStrategy creates a strategy for deep-dive queries.
// It uses the general pipeline — prompt builder will adjust instructions
// to produce comprehensive, detailed answers.
func NewTopicDeepDiveStrategy(retrieve RetrieveContextUsecase, logger *slog.Logger) RetrievalStrategy {
	return &topicDeepDiveStrategy{retrieve: retrieve, logger: logger}
}

func (s *topicDeepDiveStrategy) Name() string { return "topic_deep_dive" }

func (s *topicDeepDiveStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	s.logger.Info("deep_dive_strategy_execute",
		slog.String("query", input.Query))

	return s.retrieve.Execute(ctx, input)
}
