package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"rag-orchestrator/internal/domain"

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

	promptData, err := u.buildPrompt(ctx, input)
	if err != nil {
		slog.Warn("failed to prepare prompt for RAG answer", slog.String("retrieval_set_id", promptData.retrievalSetID), slog.String("reason", err.Error()))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, err.Error())
	}

	llmResp, err := u.llmClient.Generate(ctx, promptData.prompt, promptData.maxTokens)
	if err != nil {
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("llm generation failed: %v", err))
	}
	if llmResp == nil || strings.TrimSpace(llmResp.Text) == "" {
		slog.Warn("llm returned empty response", slog.String("retrieval_set_id", promptData.retrievalSetID), slog.Int("context_count", len(promptData.contexts)))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, "empty llm response")
	}
	if !llmResp.Done {
		slog.Warn("llm response incomplete", slog.String("retrieval_set_id", promptData.retrievalSetID))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, "llm response incomplete")
	}

	parsed, err := u.validator.Validate(llmResp.Text, promptData.contexts)
	if err != nil {
		slog.Warn("llm response validation failed", slog.String("retrieval_set_id", promptData.retrievalSetID), slog.String("error", err.Error()))
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, fmt.Sprintf("validation failed: %v", err))
	}
	if parsed.Fallback || strings.TrimSpace(parsed.Answer) == "" {
		reason := parsed.Reason
		if reason == "" {
			reason = "model signaled fallback"
		}
		return u.prepareFallback(promptData.contexts, promptData.retrievalSetID, reason)
	}

	citations := u.buildCitations(promptData.contexts, parsed.Citations)

	return &AnswerWithRAGOutput{
		Answer:    strings.TrimSpace(parsed.Answer),
		Citations: citations,
		Contexts:  promptData.contexts,
		Fallback:  false,
		Reason:    "",
		Debug: AnswerDebug{
			RetrievalSetID: promptData.retrievalSetID,
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

type promptBuildResult struct {
	retrievalSetID string
	contexts       []ContextItem
	prompt         string
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

	prompt, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return result, fmt.Errorf("failed to build prompt: %v", err)
	}

	result.prompt = prompt
	return result, nil
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

		chunkCh, errCh, err := u.llmClient.GenerateStream(ctx, promptData.prompt, promptData.maxTokens)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("llm stream setup failed: %v", err),
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
							// Look for "answer": (ignoring whitespace is tricky without regex, but we assume standard formatting or just scan)
							// We search for "answer" literal and associated structure
							// Ideally we should track JSON structure depth, but for now we scan for the unique key "answer"
							// We search starting from scanOffset to avoid re-scanning too much, but need to be careful of broken tokens
							// Actually, searching from 0 or close to end is safer? No, search from scanOffset.

							searchArea := fullStr[scanOffset:]
							idx := strings.Index(searchArea, "\"answer\"")
							if idx != -1 {
								// Found "answer", now look for colon and opening quote
								absoluteIdx := scanOffset + idx + 8 // length of "answer" + quotes = 8
								remainder := fullStr[absoluteIdx:]

								// fast forward through whitespace/colon
								startQuoteIdx := -1
								for i, r := range remainder {
									if r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == ':' {
										continue
									}
									if r == '"' {
										startQuoteIdx = absoluteIdx + i + 1 // +1 to skip the quote itself
										break
									}
									// If we hit anything else (like another key or number??), abort this finding
									break
								}

								if startQuoteIdx != -1 {
									inAnswer = true
									scanOffset = startQuoteIdx
								} else {
									// Found "answer" but not the value start yet, wait for more data
									// update scanOffset up to the "answer" start to avoid rescanning previous junk
									scanOffset += idx
								}
							} else {
								// Not found, move scanOffset but keep a buffer for split keywords
								if len(searchArea) > 20 {
									scanOffset += len(searchArea) - 20
								}
							}
						}

						if inAnswer {
							// We are inside the answer string. Scan for content.
							strToScan := fullStr[scanOffset:]
							var contentBuilder strings.Builder

							// Iterate using range to handle UTF-8 byte offsets correctly
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
									// End of answer
									inAnswer = false
									answerCompletelyStreamed = true
									advanceBytes = i + charLen // consume closing quote
									break
								}

								contentBuilder.WriteRune(char)
								advanceBytes = i + charLen
							}

							// If we didn't finish the answer, we consumed all bytes except possibly a trailing backslash
							if !answerCompletelyStreamed {
								// Re-check logic: range loop finishes. advanceBytes should be len(strToScan).
								// UNLESS the last char was backslash.
								// If last char is backslash: i points to it. isEscaped becomes true. advanceBytes becomes end.
								// contentBuilder didn't write it.
								// We need to 'unconsume' it to let next chunk handle the escape.

								// Actually, simpler logic:
								// If isEscaped is true at the end of loop, it means the last char was '\'.
								// We should NOT consume it.
								if isEscaped {
									// The last char was '\'. we want to process it again with next chunk.
									// Reduce advanceBytes by 1 (len of '\')
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

		parsed, err := u.validator.Validate(builder.String(), promptData.contexts)
		if err != nil {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("validation failed: %v", err),
			})
			return
		}

		if parsed.Fallback || strings.TrimSpace(parsed.Answer) == "" {
			reason := parsed.Reason
			if reason == "" {
				reason = "model signaled fallback"
			}
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: reason,
			})
			return
		}

		citations := u.buildCitations(promptData.contexts, parsed.Citations)
		output := &AnswerWithRAGOutput{
			Answer:    strings.TrimSpace(parsed.Answer),
			Citations: citations,
			Contexts:  promptData.contexts,
			Fallback:  false,
			Reason:    "",
			Debug: AnswerDebug{
				RetrievalSetID: promptData.retrievalSetID,
				PromptVersion:  u.promptVersion,
			},
		}

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
