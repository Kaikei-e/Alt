package eino

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"rag-orchestrator/internal/domain"

	"github.com/cloudwego/eino/schema"
	ollamaModel "github.com/cloudwego/eino-ext/components/model/ollama"
)

// ChatModelAdapter adapts Eino's Ollama ChatModel to the domain.LLMClient interface.
// This enables using Eino's framework-level features (callbacks, middleware, structured output)
// while keeping the domain layer framework-agnostic.
type ChatModelAdapter struct {
	model   *ollamaModel.ChatModel
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

	resp, err := a.model.Generate(ctx, einoMsgs)
	if err != nil {
		return nil, fmt.Errorf("eino chat: %w", err)
	}

	return &domain.LLMResponse{
		Text: resp.Content,
		Done: true,
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

	streamReader, err := a.model.Stream(ctx, einoMsgs)
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
				if readErr.Error() == "EOF" || strings.Contains(readErr.Error(), "EOF") {
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
