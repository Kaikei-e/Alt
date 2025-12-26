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

// Execute performs the 2-stage RAG generation with caching.
func (u *answerWithRAGUsecase) Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	// 1. Check Cache
	cacheKey := u.generateCacheKey(input)
	if val, ok := u.cache.Load(cacheKey); ok {
		item := val.(cacheItem)
		if time.Now().Before(item.expiresAt) {
			slog.Info("returning cached answer", slog.String("key", cacheKey))
			return item.output, nil
		} else {
			u.cache.Delete(cacheKey)
		}
	}

	// 2. Prepare Context (Retrieval)
	// We need to retrieve contexts first to know what to prompt with.
	promptData, err := u.buildPrompt(ctx, input) // Note: buildPrompt calls retrieval
	if err != nil {
		slog.Warn("failed to prepare prompt/context", slog.String("retrieval_set_id", promptData.retrievalSetID), slog.String("reason", err.Error()))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, err.Error())
	}

	// 3. Stage 1: Citations
	// Rebuild prompt for "citations" stage
	promptInput := PromptInput{
		Query:         input.Query,
		Locale:        u.defaultLocale, // simplified, real usage should respect input
		PromptVersion: u.promptVersion,
		Contexts:      u.toPromptContexts(promptData.contexts),
		Stage:         "citations",
	}
	if input.Locale != "" {
		promptInput.Locale = input.Locale
	}

	citationMessages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, "failed to build citation prompt")
	}

	citationResp, err := u.llmClient.Chat(ctx, citationMessages, promptData.maxTokens)
	if err != nil {
		// If stage 1 fails, we could fallback or try stage 2 directly?
		// Let's fallback for now as per robust design.
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("stage 1 (citations) failed: %v", err))
	}

	// Validate/Parse Citations
	parsedCitations, err := u.validator.Validate(citationResp.Text, promptData.contexts)
	if err != nil {
		// Proceed with empty citations if parsing fails? Or fallback?
		slog.Warn("stage 1 validation failed", slog.String("error", err.Error()))
		// We continue to stage 2 but with no citations.
	}

	extractCitations := []string{}
	// Convert parsed citations to string representation for Stage 2 prompt.
	// We use the "quotes" array from validation result which captures the text.
	for _, q := range parsedCitations.Quotes {
		extractCitations = append(extractCitations, fmt.Sprintf("Chunk [%s]: %s", q.ChunkID, q.Quote))
	}
	// Fallback/Enhancement: If no quotes, maybe use citations refs?
	if len(extractCitations) == 0 {
		for _, c := range parsedCitations.Citations {
			extractCitations = append(extractCitations, fmt.Sprintf("Chunk [%s] referenced", c.ChunkID))
		}
	}

	// 4. Stage 2: Answer
	promptInput.Stage = "answer"
	promptInput.Citations = extractCitations
	answerMessages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, "failed to build answer prompt")
	}

	answerResp, err := u.llmClient.Chat(ctx, answerMessages, promptData.maxTokens)
	if err != nil {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("stage 2 (answer) failed: %v", err))
	}

	// Validate Answer
	parsedAnswer, err := u.validator.Validate(answerResp.Text, promptData.contexts)
	if err != nil {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("stage 2 validation failed: %v", err))
	}

	if parsedAnswer.Fallback {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, parsedAnswer.Reason)
	}

	// Combine results
	// We use Stage 2 Answer.
	// For Citations, we should use Stage 1 Citations?
	// Or Stage 2 logic if it returns citations?
	// Prompt for Stage 2 says "Do not return quotes or citations arrays again".
	// So we must use Stage 1 Citations.
	finalCitations := u.buildCitations(promptData.contexts, parsedCitations.Citations)

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

		// 3. Stage 1: Citations (blocking for simplicity)
		promptInput := PromptInput{
			Query:         input.Query,
			Locale:        u.defaultLocale,
			PromptVersion: u.promptVersion,
			Contexts:      u.toPromptContexts(promptData.contexts),
			Stage:         "citations",
		}
		if input.Locale != "" {
			promptInput.Locale = input.Locale
		}

		citationMessages, err := u.promptBuilder.Build(promptInput)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: "failed to build citation prompt",
			})
			return
		}

		citationResp, err := u.llmClient.Chat(ctx, citationMessages, promptData.maxTokens)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("stage 1 failed: %v", err),
			})
			return
		}

		parsedCitations, err := u.validator.Validate(citationResp.Text, promptData.contexts)
		if err != nil {
			slog.Warn("stage 1 validation failed", slog.String("error", err.Error()))
		}

		extractCitations := []string{}
		for _, q := range parsedCitations.Quotes {
			extractCitations = append(extractCitations, fmt.Sprintf("Chunk [%s]: %s", q.ChunkID, q.Quote))
		}
		if len(extractCitations) == 0 {
			for _, c := range parsedCitations.Citations {
				extractCitations = append(extractCitations, fmt.Sprintf("Chunk [%s] referenced", c.ChunkID))
			}
		}

		// 4. Stage 2: Answer (Streaming)
		promptInput.Stage = "answer"
		promptInput.Citations = extractCitations
		answerMessages, err := u.promptBuilder.Build(promptInput)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: "failed to build answer prompt",
			})
			return
		}

		chunkCh, errCh, err := u.llmClient.ChatStream(ctx, answerMessages, promptData.maxTokens)
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

		// Simple streaming of Markdown content (assuming we asked for JSON but LLM might just return Markdown string inside JSON)
		// Wait, prompt asks for JSON: {"answer": "...", ...}
		// So we STILL need the partial parsing logic if LLM returns JSON.
		// Ollama Chat with Format: json WILL return JSON.
		// So I must keep the parsing logic.

		// Parsing state (copied from existing logic)
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

					// Re-use Partial Parsing Logic (simplified copy for brevity, ideal to refactor to helper)
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

		// Build Final Output (using Citations from Stage 1)
		finalCitations := u.buildCitations(promptData.contexts, parsedCitations.Citations)

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
