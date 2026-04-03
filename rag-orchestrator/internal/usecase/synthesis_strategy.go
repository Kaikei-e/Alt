package usecase

import (
	"context"
	"log/slog"
)

// SynthesisStrategy implements RetrievalStrategy for IntentSynthesis queries.
// It uses ToolPlanner to generate a tool execution plan, ToolDispatcher to
// execute tools in parallel, and falls back to standard retrieval for
// vector/keyword search context.
type SynthesisStrategy struct {
	planner    *ToolPlanner
	dispatcher *ToolDispatcher
	retrieve   RetrieveContextUsecase
	logger     *slog.Logger
}

// NewSynthesisStrategy creates a new synthesis strategy.
func NewSynthesisStrategy(
	planner *ToolPlanner,
	dispatcher *ToolDispatcher,
	retrieve RetrieveContextUsecase,
	logger *slog.Logger,
) *SynthesisStrategy {
	return &SynthesisStrategy{
		planner:    planner,
		dispatcher: dispatcher,
		retrieve:   retrieve,
		logger:     logger,
	}
}

func (s *SynthesisStrategy) Name() string { return "synthesis" }

// Retrieve executes a tool-augmented retrieval pipeline:
// 1. Generate tool plan via LLM
// 2. Execute tools in parallel (tag discovery, recap search, etc.)
// 3. Run standard vector+BM25 retrieval as baseline
// 4. Merge tool supplementary info with retrieved contexts
func (s *SynthesisStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	// 1. Generate tool plan
	plan, err := s.planner.Plan(ctx, input.Query)
	if err != nil {
		s.log("synthesis_planner_error", slog.String("error", err.Error()))
		return s.fallbackRetrieve(ctx, input, intent)
	}

	s.log("synthesis_plan_ready", slog.Int("steps", len(plan.Steps)))

	// 2. Execute tools
	toolResults := s.dispatcher.ExecutePlan(ctx, plan)

	// 3. Collect supplementary info and tool names from results
	var supplementary []string
	var toolsUsed []string
	for _, r := range toolResults {
		if r.Success && r.Data != "" {
			supplementary = append(supplementary, r.Data)
		}
		toolsUsed = append(toolsUsed, r.ToolName)
	}

	// 4. Run standard retrieval for vector + BM25 context
	baseOutput, err := s.retrieve.Execute(ctx, input)
	if err != nil {
		s.log("synthesis_base_retrieval_error", slog.String("error", err.Error()))
		// Return tool results even if base retrieval fails
		return &RetrieveContextOutput{
			SupplementaryInfo: supplementary,
			ToolsUsed:         toolsUsed,
		}, nil
	}

	// 5. Merge: base contexts + tool supplementary info
	baseOutput.SupplementaryInfo = append(baseOutput.SupplementaryInfo, supplementary...)
	baseOutput.ToolsUsed = toolsUsed

	s.log("synthesis_retrieval_complete",
		slog.Int("contexts", len(baseOutput.Contexts)),
		slog.Int("supplementary", len(supplementary)),
		slog.Int("tools", len(toolsUsed)))

	return baseOutput, nil
}

func (s *SynthesisStrategy) fallbackRetrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	s.log("synthesis_fallback_to_general")
	return s.retrieve.Execute(ctx, input)
}

func (s *SynthesisStrategy) log(msg string, attrs ...slog.Attr) {
	if s.logger == nil {
		return
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	s.logger.Info(msg, args...)
}
