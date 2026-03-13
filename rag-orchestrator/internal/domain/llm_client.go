package domain

import "context"

// LLMClient defines the capability to send prompts to an LLM and receive textual responses.
type LLMClient interface {
	Generate(ctx context.Context, prompt string, maxTokens int) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan LLMStreamChunk, <-chan error, error)
	Chat(ctx context.Context, messages []Message, maxTokens int) (*LLMResponse, error)
	ChatStream(ctx context.Context, messages []Message, maxTokens int) (<-chan LLMStreamChunk, <-chan error, error)
	Version() string
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMResponse carries the LLM output and whether the generation finished.
type LLMResponse struct {
	Text string
	Done bool
}

// LLMStreamChunk represents a single streaming response chunk returned by the LLM.
type LLMStreamChunk struct {
	Response        string
	Thinking        string
	Model           string
	Done            bool
	DoneReason      string
	PromptEvalCount *int
	EvalCount       *int
	TotalDuration   *int64
}

// QueryExpander defines the capability to expand a user query into multiple search variations.
type QueryExpander interface {
	// ExpandQuery generates search query variations from the input query.
	// japaneseCount: number of Japanese query variations to generate
	// englishCount: number of English query variations to generate
	ExpandQuery(ctx context.Context, query string, japaneseCount, englishCount int) ([]string, error)

	// ExpandQueryWithHistory generates search query variations with conversation context.
	// The LLM resolves coreferences (e.g., "tell me more about that") using the history
	// before generating search variations. This merges query rewriting and expansion
	// into a single fast LLM call (4B model) instead of a separate slow rewrite (12B model).
	ExpandQueryWithHistory(ctx context.Context, query string, history []Message, japaneseCount, englishCount int) ([]string, error)
}
