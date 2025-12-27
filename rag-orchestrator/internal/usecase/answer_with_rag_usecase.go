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
	Stream(ctx context.Context, input AnswerWithRAGInput) <-chan StreamEvent
}

type StreamEventKind string

const (
	StreamEventKindMeta     StreamEventKind = "meta"
	StreamEventKindDelta    StreamEventKind = "delta"
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

	u.logger.Info("prompt_built",
		slog.String("request_id", requestID),
		slog.Int("chunks_used", len(promptData.contexts)),
		slog.Int("prompt_size_chars", promptSize),
		slog.Int("max_tokens", promptData.maxTokens))

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
			slog.Int("contexts_available", len(promptData.contexts)))
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
			RetrievalSetID: promptData.retrievalSetID,
			PromptVersion:  u.promptVersion,
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
		meta, ok := ctxMap[cite.ChunkID]
		if !ok {
			continue
		}
		citations = append(citations, Citation{
			ChunkID:         cite.ChunkID,
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
	retrievalSetID string
	contexts       []ContextItem
	messages       []domain.Message
	maxTokens      int
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
	if len(contexts) > maxChunks {
		contexts = contexts[:maxChunks]
	}
	result.contexts = contexts

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

func (u *answerWithRAGUsecase) Stream(ctx context.Context, input AnswerWithRAGInput) <-chan StreamEvent {
	events := make(chan StreamEvent, 4)
	go func() {
		defer close(events)

		if strings.TrimSpace(input.Query) == "" {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindError,
				Payload: "query is required",
			})
			return
		}

		// 1. Check Cache (Simulated Stream)
		cacheKey := u.generateCacheKey(input)
		if val, ok := u.cache.Load(cacheKey); ok {
			item := val.(cacheItem)
			if time.Now().Before(item.expiresAt) {
				slog.Info("streaming cached answer", slog.String("key", cacheKey))
				// Emit Meta
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind: StreamEventKindMeta,
					Payload: StreamMeta{
						Contexts: item.output.Contexts,
						Debug:    item.output.Debug,
					},
				})
				// Emit Answer as a single large delta (or chunk it?)
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindDelta,
					Payload: item.output.Answer,
				})
				// Emit Done
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindDone,
					Payload: item.output,
				})
				return
			} else {
				u.cache.Delete(cacheKey)
			}
		}

		// 2. Prepare Context
		promptData, err := u.buildPrompt(ctx, input)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: err.Error(),
			})
			return
		}

		meta := StreamMeta{
			Contexts: promptData.contexts,
			Debug: AnswerDebug{
				RetrievalSetID: promptData.retrievalSetID,
				PromptVersion:  u.promptVersion,
			},
		}
		if !u.sendStreamEvent(ctx, events, StreamEvent{Kind: StreamEventKindMeta, Payload: meta}) {
			return
		}

		// 3. Single Stage Generation (Streaming)
		promptInput := PromptInput{
			Query:         input.Query,
			Locale:        u.defaultLocale,
			PromptVersion: u.promptVersion,
			Contexts:      u.toPromptContexts(promptData.contexts),
		}
		if input.Locale != "" {
			promptInput.Locale = input.Locale
		}

		messages, err := u.promptBuilder.Build(promptInput)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: "failed to build prompt",
			})
			return
		}

		chunkCh, errCh, err := u.llmClient.ChatStream(ctx, messages, promptData.maxTokens)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("llm chat stream setup failed: %v", err),
			})
			return
		}

		var builder strings.Builder
		hasData := false
		chunkStream := chunkCh
		errStream := errCh
		done := false

		// Parsing state
		scanOffset := 0
		inAnswer := false
		isEscaped := false
		answerCompletelyStreamed := false

		for {
			if chunkStream == nil && errStream == nil {
				break
			}
			select {
			case <-ctx.Done():
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindError,
					Payload: "client disconnected",
				})
				return
			case chunk, ok := <-chunkStream:
				if !ok {
					chunkStream = nil
					continue
				}
				if chunk.Response != "" {
					hasData = true
					builder.WriteString(chunk.Response)

					// Partial Parsing Logic
					if !answerCompletelyStreamed {
						fullStr := builder.String()
						if !inAnswer {
							searchArea := fullStr[scanOffset:]
							idx := strings.Index(searchArea, "\"answer\"")
							if idx != -1 {
								absoluteIdx := scanOffset + idx + 8
								remainder := fullStr[absoluteIdx:]
								startQuoteIdx := -1
								for i, r := range remainder {
									if r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == ':' {
										continue
									}
									if r == '"' {
										startQuoteIdx = absoluteIdx + i + 1
										break
									}
									break
								}
								if startQuoteIdx != -1 {
									inAnswer = true
									scanOffset = startQuoteIdx
								} else {
									scanOffset += idx
								}
							} else {
								if len(searchArea) > 20 {
									scanOffset += len(searchArea) - 20
								}
							}
						}
						if inAnswer {
							strToScan := fullStr[scanOffset:]
							var contentBuilder strings.Builder
							advanceBytes := 0
							for i, char := range strToScan {
								charLen := len(string(char))
								if isEscaped {
									isEscaped = false
									switch char {
									case 'n':
										contentBuilder.WriteRune('\n')
									case 'r':
										contentBuilder.WriteRune('\r')
									case 't':
										contentBuilder.WriteRune('\t')
									case '"':
										contentBuilder.WriteRune('"')
									case '\\':
										contentBuilder.WriteRune('\\')
									default:
										contentBuilder.WriteRune('\\')
										contentBuilder.WriteRune(char)
									}
									advanceBytes = i + charLen
									continue
								}
								if char == '\\' {
									isEscaped = true
									advanceBytes = i + charLen
									continue
								}
								if char == '"' {
									inAnswer = false
									answerCompletelyStreamed = true
									advanceBytes = i + charLen
									break
								}
								contentBuilder.WriteRune(char)
								advanceBytes = i + charLen
							}
							if !answerCompletelyStreamed {
								if isEscaped {
									advanceBytes -= 1
								}
							}
							strToStream := contentBuilder.String()
							if strToStream != "" {
								if !u.sendStreamEvent(ctx, events, StreamEvent{
									Kind:    StreamEventKindDelta,
									Payload: strToStream,
								}) {
									return
								}
							}
							scanOffset += advanceBytes
						}
					}
				}
				if chunk.Done {
					done = true
					chunkStream = nil
				}
			case streamErr, ok := <-errStream:
				if !ok {
					errStream = nil
					continue
				}
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindFallback,
					Payload: fmt.Sprintf("llm stream failed: %v", streamErr),
				})
				return
			}
			if done {
				break
			}
		}

		if !hasData {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: "llm stream produced no data",
			})
			return
		}

		// Final Validation
		parsedAnswer, err := u.validator.Validate(builder.String(), promptData.contexts)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("validation failed: %v", err),
			})
			return
		}

		if parsedAnswer.Fallback {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: parsedAnswer.Reason,
			})
			return
		}

		// Build Final Output (Hydration)
		finalCitations := u.buildCitations(promptData.contexts, parsedAnswer.Citations)

		output := &AnswerWithRAGOutput{
			Answer:    strings.TrimSpace(parsedAnswer.Answer),
			Citations: finalCitations,
			Contexts:  promptData.contexts,
			Fallback:  false,
			Reason:    "",
			Debug: AnswerDebug{
				RetrievalSetID: promptData.retrievalSetID,
				PromptVersion:  u.promptVersion,
			},
		}

		// Store in Cache
		u.cache.Store(cacheKey, cacheItem{
			output:    output,
			expiresAt: time.Now().Add(1 * time.Hour),
		})

		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindDone,
			Payload: output,
		})
	}()

	return events
}

func (u *answerWithRAGUsecase) sendStreamEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
