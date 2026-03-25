package usecase

import (
	"context"
	"log/slog"
)

type temporalStrategy struct {
	retrieve RetrieveContextUsecase
	logger   *slog.Logger
}

// NewTemporalStrategy creates a strategy optimized for temporal queries.
// It uses the general retrieval pipeline but signals temporal intent
// so that temporal boost factors are applied more aggressively.
func NewTemporalStrategy(retrieve RetrieveContextUsecase, logger *slog.Logger) RetrievalStrategy {
	return &temporalStrategy{retrieve: retrieve, logger: logger}
}

func (s *temporalStrategy) Name() string { return "temporal" }

func (s *temporalStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	s.logger.Info("temporal_strategy_execute",
		slog.String("query", input.Query))

	// Use general retrieval — temporal boost is already applied in the pipeline
	// based on article publish dates. The temporal strategy signals intent
	// so prompt builder can emphasize recency.
	return s.retrieve.Execute(ctx, input)
}
