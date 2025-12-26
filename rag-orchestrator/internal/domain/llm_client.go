package domain

import "context"

// LLMClient defines the capability to send prompts to an LLM and receive textual responses.
type LLMClient interface {
	Generate(ctx context.Context, prompt string, maxTokens int) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan LLMStreamChunk, <-chan error, error)
	Version() string
}

// LLMResponse carries the LLM output and whether the generation finished.
type LLMResponse struct {
	Text string
	Done bool
}

// LLMStreamChunk represents a single streaming response chunk returned by the LLM.
type LLMStreamChunk struct {
	Response        string
	Model           string
	Done            bool
	DoneReason      string
	PromptEvalCount *int
	EvalCount       *int
	TotalDuration   *int64
}
