package domain

import "context"

// LLMClient defines the capability to send prompts to an LLM and receive textual responses.
type LLMClient interface {
	Generate(ctx context.Context, prompt string, maxTokens int) (*LLMResponse, error)
	Version() string
}

// LLMResponse carries the LLM output and whether the generation finished.
type LLMResponse struct {
	Text string
	Done bool
}
