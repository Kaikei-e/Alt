package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
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
		if val, ok := u.cache.Get(cacheKey); ok {
			slog.InfoContext(ctx, "streaming cached answer", slog.String("key", cacheKey))
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventKindMeta,
				Payload: StreamMeta{
					Contexts: val.Contexts,
					Debug:    val.Debug,
				},
			})
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindDelta,
				Payload: val.Answer,
			})
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindDone,
				Payload: val,
			})
			return
		}

		// 2. Send immediate thinking event (Cloudflare 524 prevention)
		// This ensures HTTP response starts before long-running retrieval.
		// Cloudflare has a 60-second idle timeout on streaming connections.
		// Without this, buildPrompt (which can take 50+ seconds for retrieval + reranking)
		// would leave the stream idle, risking RST_STREAM with INTERNAL_ERROR.
		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindThinking,
			Payload: "", // Empty thinking = "processing started"
		}) {
			return
		}

		// 3. Prepare Context (retrieval + reranking)
		// Run buildPrompt in a goroutine with a heartbeat ticker to keep data
		// flowing through Cloudflare Tunnel. Without this, Cloudflare's 30-second
		// Proxy Write Timeout kills the connection during retrieval + reranking.
		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "searching",
		}) {
			return
		}

		type buildResult struct {
			data *promptBuildResult
			err  error
		}
		buildCh := make(chan buildResult, 1)
		go func() {
			data, buildErr := u.buildPrompt(ctx, input)
			buildCh <- buildResult{data: data, err: buildErr}
		}()

		var promptData *promptBuildResult
		heartbeat := time.NewTicker(u.heartbeatInterval)
		defer heartbeat.Stop()
	waitBuild:
		for {
			select {
			case <-ctx.Done():
				return
			case result := <-buildCh:
				if result.err != nil {
					u.sendStreamEvent(ctx, events, StreamEvent{
						Kind:    StreamEventKindFallback,
						Payload: result.err.Error(),
					})
					return
				}
				promptData = result.data
				break waitBuild
			case <-heartbeat.C:
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindHeartbeat,
					Payload: "",
				})
			}
		}

		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "generating",
		}) {
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

		// 4. Single Stage Generation (Streaming)
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

		// Run ChatStream in a goroutine so we can send heartbeats while
		// waiting for Ollama to accept the connection and start generating.
		type chatStreamResult struct {
			chunkCh <-chan domain.LLMStreamChunk
			errCh   <-chan error
			err     error
		}
		chatStreamCh := make(chan chatStreamResult, 1)
		go func() {
			ch, ech, setupErr := u.llmClient.ChatStream(ctx, messages, promptData.maxTokens)
			chatStreamCh <- chatStreamResult{chunkCh: ch, errCh: ech, err: setupErr}
		}()

		var chunkCh <-chan domain.LLMStreamChunk
		var errCh <-chan error
	waitChatStream:
		for {
			select {
			case <-ctx.Done():
				return
			case result := <-chatStreamCh:
				if result.err != nil {
					u.sendStreamEvent(ctx, events, StreamEvent{
						Kind:    StreamEventKindFallback,
						Payload: fmt.Sprintf("llm chat stream setup failed: %v", result.err),
					})
					return
				}
				chunkCh = result.chunkCh
				errCh = result.errCh
				break waitChatStream
			case <-heartbeat.C:
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindHeartbeat,
					Payload: "",
				})
			}
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

		for chunkStream != nil || errStream != nil {
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
				// Emit thinking chunk if present
				if chunk.Thinking != "" {
					if !u.sendStreamEvent(ctx, events, StreamEvent{
						Kind:    StreamEventKindThinking,
						Payload: chunk.Thinking,
					}) {
						return
					}
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
			case <-heartbeat.C:
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindHeartbeat,
					Payload: "",
				})
			}
			if done {
				break
			}
		}

		// Debug: Log stream loop exit state
		u.logger.Info("stream_loop_exited",
			slog.Bool("done", done),
			slog.Bool("hasData", hasData),
			slog.Int("builder_len", builder.Len()),
			slog.Bool("answerCompletelyStreamed", answerCompletelyStreamed))

		if !hasData {
			u.logger.Warn("stream_no_data_fallback",
				slog.String("retrieval_set_id", promptData.retrievalSetID))
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: "llm stream produced no data",
			})
			return
		}

		u.logger.Info("stream_proceeding_to_validation",
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.Int("builder_len", builder.Len()))

		// Final Validation
		rawResponse := builder.String()
		u.logger.Info("stream_validation_starting",
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.Int("raw_response_length", len(rawResponse)),
			slog.String("raw_response_preview", truncate(rawResponse, 500)))

		parsedAnswer, err := u.validator.Validate(rawResponse, promptData.contexts)
		if err != nil {
			u.logger.Error("stream_validation_failed",
				slog.String("retrieval_set_id", promptData.retrievalSetID),
				slog.String("error", err.Error()),
				slog.String("raw_response", truncate(rawResponse, 1000)))
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("validation failed: %v", err),
			})
			return
		}

		u.logger.Info("stream_validation_completed",
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.Bool("fallback", parsedAnswer.Fallback),
			slog.String("reason", parsedAnswer.Reason),
			slog.Int("answer_length", len(parsedAnswer.Answer)),
			slog.Int("citations_count", len(parsedAnswer.Citations)))

		if parsedAnswer.Fallback {
			u.logger.Warn("stream_answer_fallback_triggered",
				slog.String("retrieval_set_id", promptData.retrievalSetID),
				slog.String("reason", parsedAnswer.Reason),
				slog.Int("contexts_available", len(promptData.contexts)),
				slog.String("llm_raw_response", truncate(builder.String(), 500)))
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
		u.cache.Add(cacheKey, output)

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
