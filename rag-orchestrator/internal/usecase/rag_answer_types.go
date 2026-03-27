package usecase

import (
	"context"

	"rag-orchestrator/internal/domain"
)

// AnswerWithRAGInput encapsulates the parameters that drive a RAG answer request.
type AnswerWithRAGInput struct {
	Query               string
	CandidateArticleIDs []string
	MaxChunks           int
	MaxTokens           int
	UserID              string
	Locale              string
	ConversationHistory []domain.Message // Recent chat turns for multi-turn context
}

// AnswerWithRAGOutput represents the normalized answer response returned to API clients.
type AnswerWithRAGOutput struct {
	Answer           string
	Citations        []Citation
	Contexts         []ContextItem
	Fallback         bool
	Reason           string
	FallbackCategory FallbackCategory // Structured fallback reason for observability
	Debug            AnswerDebug
}

// Citation connects a chunk-level citation to the metadata needed by callers.
type Citation struct {
	ChunkID         string
	ChunkText       string
	URL             string
	Title           string
	Score           float32
	DocumentVersion int
}

// AnswerDebug surfaces metadata that aids troubleshooting and golden-test matching.
type AnswerDebug struct {
	RetrievalSetID    string
	PromptVersion     string
	ExpandedQueries   []string
	StrategyUsed      string
	IntentType        string      // Phase 2: classified intent type
	SubIntentType     string      // Analytical sub-intent (critique, opinion, implication)
	RetrievalQuality  string      // Phase 1: "good", "marginal", "insufficient"
	RetryCount        int         // Phase 1: number of retrieval retries (0 = no retry)
	ToolsUsed         []string    // Phase 3: tool names executed
	QualityFlags      []string    // Phase 4: answer quality check failures
	AgentSteps        []AgentStep // Phase 5: full agentic step trace
	TotalAgentStepsMs int64       // Phase 5: sum of all step durations
}

// AnswerWithRAGUsecase defines the contract for generating grounded answers.
type AnswerWithRAGUsecase interface {
	Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error)
	Stream(ctx context.Context, input AnswerWithRAGInput) <-chan StreamEvent
}

type StreamEventKind string

const (
	StreamEventKindMeta      StreamEventKind = "meta"
	StreamEventKindDelta     StreamEventKind = "delta"
	StreamEventKindThinking  StreamEventKind = "thinking"
	StreamEventKindProgress  StreamEventKind = "progress"
	StreamEventKindHeartbeat StreamEventKind = "heartbeat"
	StreamEventKindDone      StreamEventKind = "done"
	StreamEventKindFallback  StreamEventKind = "fallback"
	StreamEventKindError     StreamEventKind = "error"
)

type StreamEvent struct {
	Kind    StreamEventKind
	Payload interface{}
}

type StreamMeta struct {
	Contexts []ContextItem
	Debug    AnswerDebug
}

// FallbackCategory classifies why a fallback was triggered, aiding observability.
type FallbackCategory string

const (
	FallbackRetrievalEmpty   FallbackCategory = "retrieval_empty"
	FallbackGenerationFailed FallbackCategory = "generation_failed"
	FallbackValidationFailed FallbackCategory = "validation_failed"
	FallbackLLMFallback      FallbackCategory = "llm_fallback"
)
