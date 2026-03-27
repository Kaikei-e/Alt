package usecase

import (
	"context"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"
)

const toolTimeout = 5 * time.Second

// ToolDispatcher selects and executes tools based on intent classification.
// Tool selection is rule-based (not LLM-driven) for reliability with Gemma3:4b.
type ToolDispatcher struct {
	tools  map[string]domain.Tool
	logger *slog.Logger
}

// NewToolDispatcher creates a new tool dispatcher with available tools.
func NewToolDispatcher(tools map[string]domain.Tool, logger *slog.Logger) *ToolDispatcher {
	return &ToolDispatcher{tools: tools, logger: logger}
}

// Dispatch selects and executes tools based on intent and query.
// Returns tool results. Errors are logged but do not block the flow.
func (d *ToolDispatcher) Dispatch(ctx context.Context, intent QueryIntent, query string) []*domain.ToolResult {
	toolNames := d.selectTools(intent)
	if len(toolNames) == 0 {
		return nil
	}

	results := make([]*domain.ToolResult, 0, len(toolNames))
	for _, name := range toolNames {
		tool, ok := d.tools[name]
		if !ok {
			continue
		}

		toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
		result, err := tool.Execute(toolCtx, map[string]string{"query": query})
		cancel()

		if err != nil {
			d.logger.Warn("tool_execution_failed",
				slog.String("tool", name),
				slog.String("error", err.Error()))
			continue
		}

		if result != nil && result.Success {
			result.ToolName = name
			d.logger.Info("tool_execution_success",
				slog.String("tool", name),
				slog.Int("data_length", len(result.Data)))
			results = append(results, result)
		}
	}

	return results
}

// DispatchForPlan selects and executes tools based on planner output.
// Falls back to intent-based selection when the planner policy is not tool_only.
func (d *ToolDispatcher) DispatchForPlan(ctx context.Context, plan *domain.PlannerOutput, intent QueryIntent, query string) []*domain.ToolResult {
	toolNames := d.selectToolsForPlan(plan, intent)
	if len(toolNames) == 0 {
		return nil
	}

	results := make([]*domain.ToolResult, 0, len(toolNames))
	for _, name := range toolNames {
		tool, ok := d.tools[name]
		if !ok {
			continue
		}

		toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
		result, err := tool.Execute(toolCtx, map[string]string{"query": query})
		cancel()

		if err != nil {
			d.logger.Warn("tool_execution_failed",
				slog.String("tool", name),
				slog.String("error", err.Error()))
			continue
		}

		if result != nil && result.Success {
			result.ToolName = name
			d.logger.Info("tool_execution_success",
				slog.String("tool", name),
				slog.Int("data_length", len(result.Data)))
			results = append(results, result)
		}
	}

	return results
}

func (d *ToolDispatcher) selectToolsForPlan(plan *domain.PlannerOutput, intent QueryIntent) []string {
	if plan.RetrievalPolicy == domain.PolicyToolOnly {
		switch plan.Operation {
		case domain.OpRelatedArticles:
			return []string{"related_articles"}
		}
	}
	// Fallback to existing intent-based selection
	return d.selectTools(intent)
}

// selectTools returns tool names to execute based on intent type.
func (d *ToolDispatcher) selectTools(intent QueryIntent) []string {
	switch intent.IntentType {
	case IntentTemporal:
		return []string{"date_range_filter"}
	case IntentComparison:
		return []string{"tag_search"}
	case IntentArticleScoped:
		if intent.SubIntentType == SubIntentRelatedArticles {
			return []string{"related_articles"}
		}
		return nil
	default:
		return nil
	}
}
