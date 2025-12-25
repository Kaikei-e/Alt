package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
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
	RetrievalSetID string
	PromptVersion  string
}

// AnswerWithRAGUsecase defines the contract for generating grounded answers.
type AnswerWithRAGUsecase interface {
	Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error)
}

type answerWithRAGUsecase struct {
	retrieve      RetrieveContextUsecase
	promptBuilder PromptBuilder
	llmClient     domain.LLMClient
	validator     OutputValidator
	maxChunks     int
	maxTokens     int
	promptVersion string
	defaultLocale string
}

// NewAnswerWithRAGUsecase wires together the components needed to generate a RAG answer.
func NewAnswerWithRAGUsecase(
	retrieve RetrieveContextUsecase,
	promptBuilder PromptBuilder,
	llmClient domain.LLMClient,
	validator OutputValidator,
	maxChunks, maxTokens int,
	promptVersion, defaultLocale string,
) AnswerWithRAGUsecase {
	return &answerWithRAGUsecase{
		retrieve:      retrieve,
		promptBuilder: promptBuilder,
		llmClient:     llmClient,
		validator:     validator,
		maxChunks:     maxChunks,
		maxTokens:     maxTokens,
		promptVersion: promptVersion,
		defaultLocale: defaultLocale,
	}
}

func (u *answerWithRAGUsecase) Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	maxChunks := input.MaxChunks
	if maxChunks <= 0 {
		maxChunks = u.maxChunks
	}
	maxTokens := input.MaxTokens
	if maxTokens <= 0 {
		maxTokens = u.maxTokens
	}

	retrievalSetID := uuid.NewString()

	retrieveInput := RetrieveContextInput{
		Query: input.Query,
	}
	if len(input.CandidateArticleIDs) > 0 {
		retrieveInput.CandidateArticleIDs = input.CandidateArticleIDs
	}

	retrieved, err := u.retrieve.Execute(ctx, retrieveInput)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve context: %w", err)
	}

	contexts := retrieved.Contexts
	if len(contexts) == 0 {
		return u.prepareFallback(contexts, retrievalSetID, "no context returned from retrieval")
	}
	if len(contexts) > maxChunks {
		contexts = contexts[:maxChunks]
	}

	promptContexts := make([]PromptContext, len(contexts))
	for i, ctxItem := range contexts {
		promptContexts[i] = PromptContext{
			ChunkID:         ctxItem.ChunkID.String(),
			ChunkText:       ctxItem.ChunkText,
			Title:           ctxItem.Title,
			URL:             ctxItem.URL,
			PublishedAt:     ctxItem.PublishedAt,
			Score:           ctxItem.Score,
			DocumentVersion: ctxItem.DocumentVersion,
		}
	}

	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		locale = u.defaultLocale
	}

	promptInput := PromptInput{
		Query:         input.Query,
		Locale:        locale,
		PromptVersion: u.promptVersion,
		Contexts:      promptContexts,
	}

	prompt, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return u.prepareFallback(contexts, retrievalSetID, fmt.Sprintf("failed to build prompt: %v", err))
	}

	llmResp, err := u.llmClient.Generate(ctx, prompt, maxTokens)
	if err != nil {
		return u.prepareFallback(contexts, retrievalSetID, fmt.Sprintf("llm generation failed: %v", err))
	}
	if llmResp == nil || strings.TrimSpace(llmResp.Text) == "" {
		return u.prepareFallback(contexts, retrievalSetID, "empty llm response")
	}
	if !llmResp.Done {
		return u.prepareFallback(contexts, retrievalSetID, "llm response incomplete")
	}

	parsed, err := u.validator.Validate(llmResp.Text, contexts)
	if err != nil {
		return u.prepareFallback(contexts, retrievalSetID, fmt.Sprintf("validation failed: %v", err))
	}
	if parsed.Fallback || strings.TrimSpace(parsed.Answer) == "" {
		reason := parsed.Reason
		if reason == "" {
			reason = "model signaled fallback"
		}
		return u.prepareFallback(contexts, retrievalSetID, reason)
	}

	citations := u.buildCitations(contexts, parsed.Citations)

	return &AnswerWithRAGOutput{
		Answer:    strings.TrimSpace(parsed.Answer),
		Citations: citations,
		Contexts:  contexts,
		Fallback:  false,
		Reason:    "",
		Debug: AnswerDebug{
			RetrievalSetID: retrievalSetID,
			PromptVersion:  u.promptVersion,
		},
	}, nil
}

func (u *answerWithRAGUsecase) prepareFallback(contexts []ContextItem, reqID, reason string) (*AnswerWithRAGOutput, error) {
	return &AnswerWithRAGOutput{
		Answer:    "",
		Citations: nil,
		Contexts:  contexts,
		Fallback:  true,
		Reason:    reason,
		Debug: AnswerDebug{
			RetrievalSetID: reqID,
			PromptVersion:  u.promptVersion,
		},
	}, nil
}

func (u *answerWithRAGUsecase) buildCitations(contexts []ContextItem, raw []LLMCitation) []Citation {
	ctxMap := make(map[string]ContextItem, len(contexts))
	for _, ctx := range contexts {
		ctxMap[ctx.ChunkID.String()] = ctx
	}

	var citations []Citation
	for _, cite := range raw {
		meta, ok := ctxMap[cite.ChunkID]
		if !ok {
			continue
		}
		var score float32
		if cite.Score != nil {
			score = *cite.Score
		}
		citations = append(citations, Citation{
			ChunkID:         cite.ChunkID,
			ChunkText:       meta.ChunkText,
			URL:             meta.URL,
			Title:           meta.Title,
			Score:           score,
			DocumentVersion: meta.DocumentVersion,
		})
	}

	return citations
}
