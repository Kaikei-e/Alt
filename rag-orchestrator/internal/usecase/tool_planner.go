package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"rag-orchestrator/internal/domain"
)

// ToolPlanner uses an LLM to generate a structured tool execution plan
// for complex/synthesis queries.
type ToolPlanner struct {
	llmClient      domain.LLMClient
	availableTools []domain.ToolDescriptor
	validToolNames map[string]bool
	logger         *slog.Logger
}

// NewToolPlanner creates a new LLM-based tool planner.
func NewToolPlanner(llmClient domain.LLMClient, tools []domain.ToolDescriptor, logger *slog.Logger) *ToolPlanner {
	valid := make(map[string]bool, len(tools))
	for _, t := range tools {
		valid[t.Name] = true
	}
	return &ToolPlanner{
		llmClient:      llmClient,
		availableTools: tools,
		validToolNames: valid,
		logger:         logger,
	}
}

// Plan generates a tool execution plan for the given query.
// Falls back to a default plan if the LLM fails or returns invalid JSON.
func (p *ToolPlanner) Plan(ctx context.Context, query string) (*domain.ToolPlan, error) {
	prompt := p.buildPrompt(query)

	resp, err := p.llmClient.Generate(ctx, prompt, 500)
	if err != nil {
		p.log("tool_planner_llm_error", slog.String("error", err.Error()))
		return p.defaultPlan(query), nil
	}

	plan, parseErr := p.parsePlan(resp.Text)
	if parseErr != nil {
		p.log("tool_planner_parse_error",
			slog.String("error", parseErr.Error()),
			slog.String("raw", truncateText(resp.Text, 200)))
		return p.defaultPlan(query), nil
	}

	// Filter out unknown tools
	plan = p.filterValidTools(plan)

	if len(plan.Steps) == 0 {
		p.log("tool_planner_empty_after_filter")
		return p.defaultPlan(query), nil
	}

	p.log("tool_plan_generated", slog.Int("steps", len(plan.Steps)))
	return plan, nil
}

func (p *ToolPlanner) buildPrompt(query string) string {
	var sb strings.Builder
	sb.WriteString("You are a search planning agent. Given a user question, create a tool execution plan.\n\n")
	sb.WriteString("Available tools:\n")
	for _, t := range p.availableTools {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
	}
	sb.WriteString("\nCreate a JSON plan with 3-7 steps to gather comprehensive information.\n")
	sb.WriteString("Steps without depends_on run in parallel. Steps with depends_on wait for those steps.\n\n")
	fmt.Fprintf(&sb, "User question: %s\n\n", query)
	sb.WriteString("Output ONLY valid JSON:\n")
	sb.WriteString(`{"steps":[{"tool_name":"...","params":{...},"depends_on":[]},...]}"`)
	return sb.String()
}

func (p *ToolPlanner) parsePlan(raw string) (*domain.ToolPlan, error) {
	// Try to extract JSON from the response (LLM may add text around it)
	text := strings.TrimSpace(raw)

	// Find first { and last }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	text = text[start : end+1]

	var plan domain.ToolPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}

	return &plan, nil
}

func (p *ToolPlanner) filterValidTools(plan *domain.ToolPlan) *domain.ToolPlan {
	var filtered []domain.ToolStep
	for _, step := range plan.Steps {
		if p.validToolNames[step.ToolName] {
			filtered = append(filtered, step)
		}
	}
	return &domain.ToolPlan{Steps: filtered}
}

func (p *ToolPlanner) defaultPlan(query string) *domain.ToolPlan {
	steps := []domain.ToolStep{
		{ToolName: "extract_query_tags", Params: map[string]string{"query": query}},
		{ToolName: "tag_cloud_explore", Params: map[string]string{"topic": query}},
		{ToolName: "keyword_search", Params: map[string]string{"query": query}},
	}
	// Only include tools that are actually available
	var valid []domain.ToolStep
	for _, s := range steps {
		if p.validToolNames[s.ToolName] {
			valid = append(valid, s)
		}
	}
	// Always include at least the query itself as a keyword search fallback
	if len(valid) == 0 {
		valid = []domain.ToolStep{
			{ToolName: "keyword_search", Params: map[string]string{"query": query}},
		}
	}
	return &domain.ToolPlan{Steps: valid}
}

func (p *ToolPlanner) log(msg string, attrs ...slog.Attr) {
	if p.logger == nil {
		return
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	p.logger.Info(msg, args...)
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
