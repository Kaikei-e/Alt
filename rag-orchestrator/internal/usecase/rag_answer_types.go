package usecase

import (
	"context"
	"time"
)

// AnswerWithRAGInput encapsulates the parameters that drive a RAG answer request.
type AnswerWithRAGInput struct {
	Query               string
	CandidateArticleIDs []string
	MaxChunks           int
	MaxTokens           int
	UserID              string
	Locale              string
}

// AnswerWithRAGOutput represents the normalized answer response returned to API clients.
type AnswerWithRAGOutput struct {
	Answer    string
	Citations []Citation
	Contexts  []ContextItem
	Fallback  bool
	Reason    string
	Debug     AnswerDebug
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
	RetrievalSetID  string
	PromptVersion   string
	ExpandedQueries []string
}

// AnswerWithRAGUsecase defines the contract for generating grounded answers.
type AnswerWithRAGUsecase interface {
	Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error)
	Stream(ctx context.Context, input AnswerWithRAGInput) <-chan StreamEvent
}

type StreamEventKind string

const (
	StreamEventKindMeta     StreamEventKind = "meta"
	StreamEventKindDelta    StreamEventKind = "delta"
	StreamEventKindThinking StreamEventKind = "thinking"
	StreamEventKindDone     StreamEventKind = "done"
	StreamEventKindFallback StreamEventKind = "fallback"
	StreamEventKindError    StreamEventKind = "error"
)

type StreamEvent struct {
	Kind    StreamEventKind
	Payload interface{}
}

type StreamMeta struct {
	Contexts []ContextItem
	Debug    AnswerDebug
}

type cacheItem struct {
	output    *AnswerWithRAGOutput
	expiresAt time.Time
}
