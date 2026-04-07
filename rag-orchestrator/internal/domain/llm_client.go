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

// ToolCallingLLMClient extends LLMClient with native tool-calling support.
type ToolCallingLLMClient interface {
	ChatWithTools(ctx context.Context, messages []Message, tools []ToolDefinition, maxTokens int) (*LLMResponse, error)
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// LLMResponse carries the LLM output and whether the generation finished.
type LLMResponse struct {
	Text      string
	ToolCalls []ToolCall
	Done      bool
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

// ToolDefinition describes a tool exposed to an LLM for native function calling.
type ToolDefinition struct {
	Type     string           `json:"type"`
	Function ToolDescriptorFn `json:"function"`
}

// ToolDescriptorFn mirrors the Ollama/OpenAI tool schema function block.
type ToolDescriptorFn struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolCall represents a model-requested tool invocation.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the requested tool name and JSON arguments.
type ToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
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
