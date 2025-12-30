package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"rag-orchestrator/internal/domain"

	"sync"
	"time"

	"github.com/google/uuid"
)

type answerWithRAGUsecase struct {
	retrieve      RetrieveContextUsecase
	promptBuilder PromptBuilder
	llmClient     domain.LLMClient
	validator     OutputValidator
	maxChunks     int
	maxTokens     int
	promptVersion string
	defaultLocale string
	cache         sync.Map // key: string hash, value: cacheItem
	logger        *slog.Logger
}

// NewAnswerWithRAGUsecase wires together the components needed to generate a RAG answer.
func NewAnswerWithRAGUsecase(
	retrieve RetrieveContextUsecase,
	promptBuilder PromptBuilder,
	llmClient domain.LLMClient,
	validator OutputValidator,
	maxChunks, maxTokens int,
	promptVersion, defaultLocale string,
	logger *slog.Logger,
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
		logger:        logger,
	}
}

// Execute performs the Single-Phase RAG generation with caching.
func (u *answerWithRAGUsecase) Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	executionStart := time.Now()
	requestID := uuid.NewString()

	u.logger.Info("answer_request_started",
		slog.String("request_id", requestID),
		slog.String("query", input.Query),
		slog.Int("max_chunks", input.MaxChunks),
		slog.String("locale", input.Locale))

	// 1. Check Cache
	cacheKey := u.generateCacheKey(input)
	if val, ok := u.cache.Load(cacheKey); ok {
		item := val.(cacheItem)
		if time.Now().Before(item.expiresAt) {
			u.logger.Info("cache_hit",
				slog.String("request_id", requestID),
				slog.String("cache_key", cacheKey),
				slog.String("query", input.Query))
			return item.output, nil
		} else {
			u.cache.Delete(cacheKey)
		}
	}

	// 2. Prepare Context (Retrieval)
	// We need to retrieve contexts first to know what to prompt with.
	promptData, err := u.buildPrompt(ctx, input) // Note: buildPrompt calls retrieval
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", err.Error()),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, err.Error())
	}

	u.logger.Info("context_retrieved",
		slog.String("request_id", requestID),
		slog.Int("contexts_count", len(promptData.contexts)),
		slog.String("retrieval_set_id", promptData.retrievalSetID))

	// Log context titles for debugging
	for i, ctx := range promptData.contexts {
		u.logger.Debug("context_chunk_detail",
			slog.String("request_id", requestID),
			slog.Int("index", i+1),
			slog.String("title", ctx.Title),
			slog.Float64("score", float64(ctx.Score)),
			slog.Int("chunk_length", len(ctx.ChunkText)))
	}

	// 3. Single Stage Generation
	promptInput := PromptInput{
		Query:         input.Query,
		Locale:        u.defaultLocale,
		PromptVersion: u.promptVersion,
		Contexts:      u.toPromptContexts(promptData.contexts),
		// Stage and Citations inputs are no longer needed for single phase
	}
	if input.Locale != "" {
		promptInput.Locale = input.Locale
	}

	messages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", "failed to build prompt"),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, "failed to build prompt")
	}

	// Calculate approximate prompt size
	promptSize := 0
	for _, msg := range messages {
		promptSize += len(msg.Content)
	}

	firstTitle := ""
	if len(promptData.contexts) > 0 {
		firstTitle = promptData.contexts[0].Title
	}

	u.logger.Info("prompt_built",
		slog.String("request_id", requestID),
		slog.Int("chunks_used", len(promptData.contexts)),
		slog.Int("prompt_size_chars", promptSize),
		slog.Int("max_tokens", promptData.maxTokens),
		slog.String("first_context_title", firstTitle),
		slog.String("query", input.Query))

	generationStart := time.Now()
	u.logger.Info("llm_generation_started",
		slog.String("request_id", requestID),
		slog.String("retrieval_set_id", promptData.retrievalSetID))

	resp, err := u.llmClient.Chat(ctx, messages, promptData.maxTokens)
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", fmt.Sprintf("generation failed: %v", err)),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("generation failed: %v", err))
	}

	generationDuration := time.Since(generationStart)
	u.logger.Info("llm_generation_completed",
		slog.String("request_id", requestID),
		slog.Int("response_length", len(resp.Text)),
		slog.Int64("generation_ms", generationDuration.Milliseconds()))

	// Validate/Parse Answer
	parsedAnswer, err := u.validator.Validate(resp.Text, promptData.contexts)
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", fmt.Sprintf("validation failed: %v", err)),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("validation failed: %v", err))
	}

	u.logger.Info("validation_completed",
		slog.String("request_id", requestID),
		slog.Bool("is_fallback", parsedAnswer.Fallback),
		slog.Int("citations_count", len(parsedAnswer.Citations)))

	if parsedAnswer.Fallback {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", parsedAnswer.Reason),
			slog.Int("contexts_available", len(promptData.contexts)),
			slog.String("llm_raw_response", truncate(resp.Text, 500)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, parsedAnswer.Reason)
	}

	// Build Citations (Hydration)
	finalCitations := u.buildCitations(promptData.contexts, parsedAnswer.Citations)

	output := &AnswerWithRAGOutput{
		Answer:    strings.TrimSpace(parsedAnswer.Answer),
		Citations: finalCitations,
		Contexts:  promptData.contexts,
		Fallback:  false,
		Reason:    "",
		Debug: AnswerDebug{
			RetrievalSetID:  promptData.retrievalSetID,
			PromptVersion:   u.promptVersion,
			ExpandedQueries: promptData.expandedQueries,
		},
	}

	// 5. Store in Cache
	u.cache.Store(cacheKey, cacheItem{
		output:    output,
		expiresAt: time.Now().Add(1 * time.Hour),
	})

	executionDuration := time.Since(executionStart)
	u.logger.Info("answer_request_completed",
		slog.String("request_id", requestID),
		slog.Int("answer_length", len(output.Answer)),
		slog.Int("citations", len(output.Citations)),
		slog.Int64("total_duration_ms", executionDuration.Milliseconds()))

	return output, nil
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
		var meta ContextItem
		var ok bool

		// 1. Try direct lookup (UUID)
		meta, ok = ctxMap[cite.ChunkID]
		if !ok {
			// 2. Try index lookup (e.g., "1", "2")
			// Used when prompt asks for [index] citations to save tokens
			var idx int
			if _, err := fmt.Sscanf(cite.ChunkID, "%d", &idx); err == nil {
				// 1-based index -> 0-based slice
				sliceIdx := idx - 1
				if sliceIdx >= 0 && sliceIdx < len(contexts) {
					meta = contexts[sliceIdx]
					ok = true
				}
			}
		}

		if !ok {
			continue
		}
		citations = append(citations, Citation{
			ChunkID:         meta.ChunkID.String(), // Always return real UUID to caller
			ChunkText:       meta.ChunkText,
			URL:             meta.URL,
			Title:           meta.Title,
			Score:           meta.Score, // Use retrieval score
			DocumentVersion: meta.DocumentVersion,
		})
	}

	return citations
}

type promptBuildResult struct {
	retrievalSetID  string
	contexts        []ContextItem
	messages        []domain.Message
	maxTokens       int
	expandedQueries []string
}

func (u *answerWithRAGUsecase) buildPrompt(ctx context.Context, input AnswerWithRAGInput) (*promptBuildResult, error) {
	maxChunks := input.MaxChunks
	if maxChunks <= 0 {
		maxChunks = u.maxChunks
	}
	maxTokens := input.MaxTokens
	if maxTokens <= 0 {
		maxTokens = u.maxTokens
	}

	result := &promptBuildResult{
		retrievalSetID: uuid.NewString(),
		maxTokens:      maxTokens,
	}

	retrieveInput := RetrieveContextInput{
		Query: input.Query,
	}
	if len(input.CandidateArticleIDs) > 0 {
		retrieveInput.CandidateArticleIDs = input.CandidateArticleIDs
	}

	retrieved, err := u.retrieve.Execute(ctx, retrieveInput)
	if err != nil {
		return result, fmt.Errorf("failed to retrieve context: %w", err)
	}

	contexts := retrieved.Contexts

	// Limit to maxChunks
	if len(contexts) > maxChunks {
		contexts = contexts[:maxChunks]
	}

	result.contexts = contexts
	result.expandedQueries = retrieved.ExpandedQueries

	if len(contexts) == 0 {
		return result, errors.New("no context returned from retrieval")
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

	messages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return result, fmt.Errorf("failed to build messages: %v", err)
	}

	result.messages = messages
	// Helper to extract PromptContexts
	return result, nil
}

func (u *answerWithRAGUsecase) toPromptContexts(contexts []ContextItem) []PromptContext {
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
	return promptContexts
}

func (u *answerWithRAGUsecase) generateCacheKey(input AnswerWithRAGInput) string {
	// Simple key generation
	return fmt.Sprintf("%s|%v|%s", input.Query, input.CandidateArticleIDs, input.Locale)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
