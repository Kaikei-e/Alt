package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Stream streams a RAG answer using Server-Sent Events.
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
