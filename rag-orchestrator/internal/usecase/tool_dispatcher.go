package usecase

import (
	"context"
	"log/slog"
	"sync"
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

// ExecutePlan runs tools according to a ToolPlan, respecting step dependencies.
// Independent steps (no depends_on) run in parallel. Dependent steps wait.
// Tool failures are captured in results but do not abort other steps.
func (d *ToolDispatcher) ExecutePlan(ctx context.Context, plan *domain.ToolPlan) []domain.ToolResult {
	n := len(plan.Steps)
	results := make([]domain.ToolResult, n)
	done := make([]bool, n)
	var mu sync.Mutex

	// Build a level-order execution: steps with no unmet dependencies run first.
	for {
		// Find runnable steps: not done, all dependencies done.
		var runnable []int
		mu.Lock()
		for i := 0; i < n; i++ {
			if done[i] {
				continue
			}
			ready := true
			for _, dep := range plan.Steps[i].DependsOn {
				if dep >= 0 && dep < n && !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				runnable = append(runnable, i)
			}
		}
		mu.Unlock()

		if len(runnable) == 0 {
			break
		}

		// Run all runnable steps in parallel.
		var wg sync.WaitGroup
		for _, idx := range runnable {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				step := plan.Steps[i]
				result := d.executeStep(ctx, step)
				mu.Lock()
				results[i] = result
				done[i] = true
				mu.Unlock()
			}(idx)
		}
		wg.Wait()
	}

	return results
}

func (d *ToolDispatcher) executeStep(ctx context.Context, step domain.ToolStep) domain.ToolResult {
	tool, ok := d.tools[step.ToolName]
	if !ok {
		return domain.ToolResult{
			ToolName: step.ToolName,
			Success:  false,
			Error:    "tool not found: " + step.ToolName,
		}
	}

	toolCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result, err := tool.Execute(toolCtx, step.Params)
	if err != nil {
		d.logger.Warn("plan_tool_execution_failed",
			slog.String("tool", step.ToolName),
			slog.String("error", err.Error()))
		return domain.ToolResult{
			ToolName: step.ToolName,
			Success:  false,
			Error:    err.Error(),
		}
	}

	if result == nil {
		return domain.ToolResult{
			ToolName: step.ToolName,
			Success:  false,
			Error:    "tool returned nil result",
		}
	}

	result.ToolName = step.ToolName
	d.logger.Info("plan_tool_executed",
		slog.String("tool", step.ToolName),
		slog.Bool("success", result.Success),
		slog.Int("data_length", len(result.Data)))

	return *result
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
