package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"rag-orchestrator/internal/domain"
)

// AgenticSynthesisStrategy extends the existing synthesis pipeline with a
// native Ollama tool-calling loop. It keeps the existing retrieval/planner
// fallback behavior, then lets the model request additional tools when it
// needs more evidence.
type AgenticSynthesisStrategy struct {
	base         *SynthesisStrategy
	toolCaller   domain.ToolCallingLLMClient
	dispatcher   *ToolDispatcher
	logger       *slog.Logger
	maxToolCalls int
}

// NewAgenticSynthesisStrategy creates a synthesis strategy with native tool calling.
func NewAgenticSynthesisStrategy(
	planner *ToolPlanner,
	dispatcher *ToolDispatcher,
	retrieve RetrieveContextUsecase,
	toolCaller domain.ToolCallingLLMClient,
	logger *slog.Logger,
) RetrievalStrategy {
	return &AgenticSynthesisStrategy{
		base:         NewSynthesisStrategy(planner, dispatcher, retrieve, logger),
		toolCaller:   toolCaller,
		dispatcher:   dispatcher,
		logger:       logger,
		maxToolCalls: 3,
	}
}

func (s *AgenticSynthesisStrategy) Name() string { return "synthesis_agentic" }

func (s *AgenticSynthesisStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	baseOutput, err := s.base.Retrieve(ctx, input, intent)
	if err != nil || baseOutput == nil {
		return baseOutput, err
	}
	if s.toolCaller == nil || s.dispatcher == nil {
		return baseOutput, nil
	}

	toolDefs := s.dispatcher.ToolDefinitions()
	if len(toolDefs) == 0 {
		return baseOutput, nil
	}

	agentSupplementary, agentTools := s.runAgentLoop(ctx, input.Query, baseOutput, toolDefs)
	if len(agentSupplementary) == 0 && len(agentTools) == 0 {
		return baseOutput, nil
	}

	supplementarySeen := make(map[string]struct{}, len(baseOutput.SupplementaryInfo))
	for _, item := range baseOutput.SupplementaryInfo {
		supplementarySeen[item] = struct{}{}
	}
	toolsSeen := make(map[string]struct{}, len(baseOutput.ToolsUsed))
	for _, item := range baseOutput.ToolsUsed {
		toolsSeen[item] = struct{}{}
	}

	addedSupplementary := 0
	for _, item := range agentSupplementary {
		if _, ok := supplementarySeen[item]; ok {
			continue
		}
		baseOutput.SupplementaryInfo = append(baseOutput.SupplementaryInfo, item)
		supplementarySeen[item] = struct{}{}
		addedSupplementary++
	}

	addedTools := 0
	for _, item := range agentTools {
		if _, ok := toolsSeen[item]; ok {
			continue
		}
		baseOutput.ToolsUsed = append(baseOutput.ToolsUsed, item)
		toolsSeen[item] = struct{}{}
		addedTools++
	}

	s.log("agentic_synthesis_completed",
		slog.Int("supplementary", addedSupplementary),
		slog.Int("tools", addedTools))
	return baseOutput, nil
}

func (s *AgenticSynthesisStrategy) runAgentLoop(
	ctx context.Context,
	query string,
	baseOutput *RetrieveContextOutput,
	toolDefs []domain.ToolDefinition,
) ([]string, []string) {
	messages := s.buildAgentMessages(query, baseOutput, toolDefs)
	var supplementary []string
	var toolsUsed []string
	seenCalls := make(map[string]struct{})

	for step := 0; step < s.maxToolCalls; step++ {
		resp, err := s.toolCaller.ChatWithTools(ctx, messages, toolDefs, 256)
		if err != nil {
			s.log("agentic_tool_chat_failed", slog.String("error", err.Error()))
			break
		}
		if resp == nil {
			break
		}

		if len(resp.ToolCalls) == 0 {
			if text := strings.TrimSpace(resp.Text); text != "" {
				supplementary = append(supplementary, text)
			}
			messages = append(messages, domain.Message{Role: "assistant", Content: resp.Text})
			break
		}

		messages = append(messages, domain.Message{
			Role:      "assistant",
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
		})

		plan := &domain.ToolPlan{Steps: make([]domain.ToolStep, 0, len(resp.ToolCalls))}
		for _, call := range resp.ToolCalls {
			name := strings.TrimSpace(call.Function.Name)
			if name == "" {
				continue
			}
			params := toolCallArgsToStrings(call.Function.Arguments)
			key := toolCallKey(name, params)
			if _, seen := seenCalls[key]; seen {
				s.log("agentic_tool_duplicate_skipped", slog.String("tool", name))
				continue
			}
			seenCalls[key] = struct{}{}
			plan.Steps = append(plan.Steps, domain.ToolStep{
				ToolName: name,
				Params:   params,
			})
		}

		if len(plan.Steps) == 0 {
			break
		}

		results := s.dispatcher.ExecutePlan(ctx, plan)
		for i, result := range results {
			step := plan.Steps[i]
			if !result.Success {
				continue
			}
			if strings.TrimSpace(result.Data) != "" {
				supplementary = append(supplementary, result.Data)
			}
			toolsUsed = append(toolsUsed, step.ToolName)
			messages = append(messages, domain.Message{
				Role:       "tool",
				Name:       step.ToolName,
				ToolCallID: toolCallKey(step.ToolName, step.Params),
				Content:    result.Data,
			})
		}
	}

	return supplementary, toolsUsed
}

func (s *AgenticSynthesisStrategy) buildAgentMessages(
	query string,
	baseOutput *RetrieveContextOutput,
	toolDefs []domain.ToolDefinition,
) []domain.Message {
	var sb strings.Builder
	sb.WriteString("You are a retrieval agent for agentic RAG.\n")
	sb.WriteString("Use the available tools only when additional evidence is needed.\n")
	sb.WriteString("Do not answer the user directly; gather evidence and return concise notes.\n")
	sb.WriteString("Stop once the evidence is sufficient or after a few tool calls.\n\n")
	sb.WriteString("Available tools:\n")
	for _, tool := range toolDefs {
		fmt.Fprintf(&sb, "- %s: %s\n", tool.Function.Name, tool.Function.Description)
	}

	if len(baseOutput.Contexts) > 0 {
		sb.WriteString("\nExisting evidence:\n")
		limit := len(baseOutput.Contexts)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			ctx := baseOutput.Contexts[i]
			fmt.Fprintf(&sb, "- %s: %s\n", ctx.Title, truncateToolText(ctx.ChunkText, 180))
		}
	}
	if len(baseOutput.SupplementaryInfo) > 0 {
		sb.WriteString("\nSupplementary evidence:\n")
		limit := len(baseOutput.SupplementaryInfo)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			fmt.Fprintf(&sb, "- %s\n", truncateToolText(baseOutput.SupplementaryInfo[i], 180))
		}
	}

	return []domain.Message{
		{Role: "system", Content: sb.String()},
		{Role: "user", Content: query},
	}
}

func toolCallArgsToStrings(args map[string]any) map[string]string {
	if len(args) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(args))
	for k, v := range args {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func toolCallKey(name string, params map[string]string) string {
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteString("|")
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := params[k]
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString(";")
	}
	return sb.String()
}

func truncateToolText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (s *AgenticSynthesisStrategy) log(msg string, attrs ...slog.Attr) {
	if s.logger == nil {
		return
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	s.logger.Info(msg, args...)
}
