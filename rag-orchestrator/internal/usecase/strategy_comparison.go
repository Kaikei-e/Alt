package usecase

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

type comparisonStrategy struct {
	retrieve RetrieveContextUsecase
	logger   *slog.Logger
}

// NewComparisonStrategy creates a strategy for comparison queries.
// It retrieves context using the general pipeline — future enhancement
// will extract comparison targets and retrieve for each independently.
func NewComparisonStrategy(retrieve RetrieveContextUsecase, logger *slog.Logger) RetrievalStrategy {
	return &comparisonStrategy{retrieve: retrieve, logger: logger}
}

func (s *comparisonStrategy) Name() string { return "comparison" }

func (s *comparisonStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	s.logger.Info("comparison_strategy_execute",
		slog.String("query", input.Query))

	// Use general retrieval with the full comparison query.
	// The query expansion step will naturally generate variants
	// covering both sides of the comparison.
	result, err := s.retrieve.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// Deduplicate by ChunkID (in case expansion hits same chunks)
	seen := make(map[uuid.UUID]bool)
	var deduped []ContextItem
	for _, c := range result.Contexts {
		if !seen[c.ChunkID] {
			deduped = append(deduped, c)
			seen[c.ChunkID] = true
		}
	}
	result.Contexts = deduped

	return result, nil
}
