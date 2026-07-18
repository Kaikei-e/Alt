package usecase

import (
	"context"
	"log/slog"
	"sort"
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
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &ToolDispatcher{tools: tools, logger: logger}
}

// ToolDefinitions converts the dispatcher tool registry into Ollama/OpenAI-style tool schemas.
func (d *ToolDispatcher) ToolDefinitions() []domain.ToolDefinition {
	defs := make([]domain.ToolDefinition, 0, len(d.tools))
	for _, tool := range d.tools {
		defs = append(defs, domain.ToolDefinition{
			Type: "function",
			Function: domain.ToolDescriptorFn{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  toolParamsForName(tool.Name()),
			},
		})
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Function.Name < defs[j].Function.Name
	})
	return defs
}

func toolParamsForName(name string) map[string]any {
	switch name {
	case "tag_cloud_explore":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{
					"type":        "string",
					"description": "Topic to explore",
				},
			},
			"required":             []string{"topic"},
			"additionalProperties": false,
		}
	case "articles_by_tag", "search_recaps":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tag_name": map[string]any{
					"type":        "string",
					"description": "Tag name to search for",
				},
			},
			"required":             []string{"tag_name"},
			"additionalProperties": false,
		}
	case "date_range_filter", "related_articles", "tag_search", "extract_query_tags":
		fallthrough
	default:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query or input",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		}
	}
}

// Dispatch selects and executes tools based on intent and query.
// Returns tool results. Errors are logged but do not block the flow.
func (d *ToolDispatcher) Dispatch(ctx context.Context, intent QueryIntent, query string) []*domain.ToolResult {
	return d.executeTools(ctx, d.selectTools(intent), query)
}

// executeTools runs the named tools sequentially, respecting ctx cancellation.
func (d *ToolDispatcher) executeTools(ctx context.Context, toolNames []string, query string) []*domain.ToolResult {
	if len(toolNames) == 0 {
		return nil
	}

	results := make([]*domain.ToolResult, 0, len(toolNames))
	for _, name := range toolNames {
		if ctx.Err() != nil {
			break
		}
		tool, ok := d.tools[name]
		if !ok {
			continue
		}

		toolCtx, cancel := context.WithTimeout(ctx, toolTimeout)
		result, err := tool.Execute(toolCtx, defaultToolArgs(name, query))
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

func defaultToolArgs(name, query string) map[string]string {
	switch name {
	case "tag_cloud_explore":
		return map[string]string{"topic": query}
	case "articles_by_tag", "search_recaps":
		return map[string]string{"tag_name": query}
	case "date_range_filter", "related_articles", "tag_search", "extract_query_tags":
		fallthrough
	default:
		return map[string]string{"query": query}
	}
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
		// A depends_on index outside [0, n) can never be satisfied — fail that
		// step outright instead of silently treating the out-of-range
		// dependency as met.
		var runnable []int
		var outOfRange []int
		mu.Lock()
		for i := 0; i < n; i++ {
			if done[i] {
				continue
			}
			ready := true
			invalidDep := false
			for _, dep := range plan.Steps[i].DependsOn {
				if dep < 0 || dep >= n {
					invalidDep = true
					break
				}
				if !done[dep] {
					ready = false
					break
				}
			}
			switch {
			case invalidDep:
				outOfRange = append(outOfRange, i)
			case ready:
				runnable = append(runnable, i)
			}
		}
		for _, i := range outOfRange {
			step := plan.Steps[i]
			d.logger.Warn("plan_step_invalid_dependency",
				slog.String("tool", step.ToolName),
				slog.Int("step", i),
				slog.Any("depends_on", step.DependsOn))
			results[i] = domain.ToolResult{
				ToolName: step.ToolName,
				Success:  false,
				Error:    "depends_on references an out-of-range step",
			}
			done[i] = true
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

	// Any step still not done at this point is stuck behind an unmet
	// dependency (e.g. a dependency cycle) — surface that instead of
	// returning a silent zero-value ToolResult.
	for i := 0; i < n; i++ {
		if done[i] {
			continue
		}
		step := plan.Steps[i]
		d.logger.Warn("plan_step_never_ran",
			slog.String("tool", step.ToolName),
			slog.Int("step", i),
			slog.Any("depends_on", step.DependsOn))
		results[i] = domain.ToolResult{
			ToolName: step.ToolName,
			Success:  false,
			Error:    "step never ran: unmet or cyclic dependency",
		}
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
