package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"rag-orchestrator/internal/domain"

	ollamaModel "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ChatModelAdapter adapts Eino's Ollama ChatModel to the domain.LLMClient interface.
// This enables using Eino's framework-level features (callbacks, middleware, structured output)
// while keeping the domain layer framework-agnostic.
type ChatModelAdapter struct {
	model   model.ToolCallingChatModel
	version string
	logger  *slog.Logger
}

// NewChatModelAdapter creates an Eino-backed LLM client.
func NewChatModelAdapter(ctx context.Context, baseURL, modelName string, logger *slog.Logger) (*ChatModelAdapter, error) {
	cm, err := ollamaModel.NewChatModel(ctx, &ollamaModel.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("eino ollama chat model init: %w", err)
	}

	return &ChatModelAdapter{
		model:   cm,
		version: "eino-ollama-" + modelName,
		logger:  logger,
	}, nil
}

// Version returns the model identifier.
func (a *ChatModelAdapter) Version() string {
	return a.version
}

// Chat sends messages to the LLM and returns the response.
func (a *ChatModelAdapter) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	einoMsgs := toEinoMessages(messages)

	resp, err := a.model.Generate(ctx, einoMsgs, model.WithMaxTokens(maxTokens))
	if err != nil {
		return nil, fmt.Errorf("eino chat: %w", err)
	}

	return &domain.LLMResponse{
		Text:      resp.Content,
		ToolCalls: toDomainToolCalls(resp.ToolCalls),
		Done:      true,
	}, nil
}

// Generate sends a single prompt to the LLM.
func (a *ChatModelAdapter) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	return a.Chat(ctx, []domain.Message{
		{Role: "user", Content: prompt},
	}, maxTokens)
}

// ChatStream sends messages and returns a streaming channel.
func (a *ChatModelAdapter) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	einoMsgs := toEinoMessages(messages)

	streamReader, err := a.model.Stream(ctx, einoMsgs, model.WithMaxTokens(maxTokens))
	if err != nil {
		return nil, nil, fmt.Errorf("eino chat stream setup: %w", err)
	}

	chunkCh := make(chan domain.LLMStreamChunk, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		for {
			msg, readErr := streamReader.Recv()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) || strings.Contains(readErr.Error(), "EOF") {
					chunkCh <- domain.LLMStreamChunk{Done: true}
					return
				}
				errCh <- readErr
				return
			}
			if msg == nil {
				chunkCh <- domain.LLMStreamChunk{Done: true}
				return
			}

			chunkCh <- domain.LLMStreamChunk{
				Response: msg.Content,
				Done:     false,
			}
		}
	}()

	return chunkCh, errCh, nil
}

// GenerateStream sends a single prompt and returns a streaming channel.
func (a *ChatModelAdapter) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	return a.ChatStream(ctx, []domain.Message{
		{Role: "user", Content: prompt},
	}, maxTokens)
}

// ChatWithTools sends a conversation with bound tools to the LLM.
func (a *ChatModelAdapter) ChatWithTools(ctx context.Context, messages []domain.Message, tools []domain.ToolDefinition, maxTokens int) (*domain.LLMResponse, error) {
	einoMsgs := toEinoMessages(messages)
	toolInfos := toEinoToolInfos(tools)
	if len(toolInfos) == 0 {
		return a.Chat(ctx, messages, maxTokens)
	}

	toolModel, err := a.model.WithTools(toolInfos)
	if err != nil {
		return nil, fmt.Errorf("eino bind tools: %w", err)
	}

	resp, err := toolModel.Generate(ctx, einoMsgs, model.WithMaxTokens(maxTokens), model.WithToolChoice(schema.ToolChoiceAllowed))
	if err != nil {
		return nil, fmt.Errorf("eino chat with tools: %w", err)
	}

	return &domain.LLMResponse{
		Text:      resp.Content,
		ToolCalls: toDomainToolCalls(resp.ToolCalls),
		Done:      true,
	}, nil
}

// toEinoMessages converts domain messages to Eino schema messages.
func toEinoMessages(msgs []domain.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		var role schema.RoleType
		switch m.Role {
		case "system":
			role = schema.System
		case "assistant":
			role = schema.Assistant
		case "tool":
			role = schema.Tool
		default:
			role = schema.User
		}

		result = append(result, &schema.Message{
			Role:    role,
			Content: m.Content,
		})
	}
	return result
}

func toDomainToolCalls(calls []schema.ToolCall) []domain.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	out := make([]domain.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, domain.ToolCall{
			Function: domain.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: parseToolCallArguments(call.Function.Arguments),
			},
		})
	}
	return out
}

func toEinoToolInfos(defs []domain.ToolDefinition) []*schema.ToolInfo {
	if len(defs) == 0 {
		return nil
	}

	infos := make([]*schema.ToolInfo, 0, len(defs))
	for _, def := range defs {
		params := toolParamsForName(def.Function.Name)
		infos = append(infos, &schema.ToolInfo{
			Name:        def.Function.Name,
			Desc:        def.Function.Description,
			ParamsOneOf: schema.NewParamsOneOfByParams(params),
		})
	}
	return infos
}
