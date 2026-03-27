package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

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

		// Clarification check: if the planner determined the query needs
		// user clarification, short-circuit before generation.
		if promptData.plannerOutput != nil && promptData.plannerOutput.NeedsClarification {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventKindClarification,
				Payload: StreamClarification{
					Message: promptData.plannerOutput.ClarificationMsg,
					Options: promptData.plannerOutput.EntityFocus,
				},
			})
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindDone,
				Payload: nil,
			})
			return
		}

		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "generating",
		}) {
			return
		}

		debug := AnswerDebug{
			RetrievalSetID:        promptData.retrievalSetID,
			PromptVersion:         u.promptVersion,
			ExpandedQueries:       promptData.expandedQueries,
			StrategyUsed:          promptData.strategyUsed,
			IntentType:            string(promptData.intentType),
			SubIntentType:         string(promptData.subIntentType),
			RetrievalQuality:      string(promptData.retrievalQuality),
			RetryCount:            promptData.retryCount,
			ToolsUsed:             promptData.toolsUsed,
			RetrievalPolicy:       promptData.retrievalPolicy,
			GeneralRetrievalGated: promptData.generalGated,
		}
		if promptData.plannerOutput != nil {
			debug.PlannerOperation = string(promptData.plannerOutput.Operation)
			debug.PlannerConfidence = promptData.plannerOutput.Confidence
		}
		meta := StreamMeta{
			Contexts: promptData.contexts,
			Debug:    debug,
		}
		if !u.sendStreamEvent(ctx, events, StreamEvent{Kind: StreamEventKindMeta, Payload: meta}) {
			return
		}

		// 4. Single Stage Generation (Streaming) — use messages from buildPrompt directly
		messages := promptData.messages

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

		var builder strings.Builder // Full response for final validation
		hasData := false
		chunkStream := chunkCh
		errStream := errCh
		done := false

		// Parsing state — O(n) incremental parser.
		// `pending` holds only unprocessed bytes (not the full accumulated text).
		var pending strings.Builder
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

					// Partial Parsing Logic — only process new chunk data
					if !answerCompletelyStreamed {
						pending.WriteString(chunk.Response)
						pendingStr := pending.String()
						processed := 0

						if !inAnswer {
							idx := strings.Index(pendingStr, "\"answer\"")
							if idx != -1 {
								// Find the opening quote of the value
								remainder := pendingStr[idx+8:]
								startOffset := -1
								for i, r := range remainder {
									if r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == ':' {
										continue
									}
									if r == '"' {
										startOffset = idx + 8 + i + len(string(r))
										break
									}
									break
								}
								if startOffset != -1 {
									inAnswer = true
									processed = startOffset
								} else {
									// Keep tail for cross-chunk matching
									if len(pendingStr) > 20 {
										processed = len(pendingStr) - 20
									}
								}
							} else {
								// Keep tail for cross-chunk matching of "answer" key
								if len(pendingStr) > 20 {
									processed = len(pendingStr) - 20
								}
							}
						}

						if inAnswer {
							strToScan := pendingStr[processed:]
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
									// Lookahead: verify this quote is a real JSON field terminator,
									// not an unescaped quote from LLM structured output bug.
									// After the answer field's closing ", the next non-whitespace
									// must be , or } to form valid JSON structure.
									tail := strToScan[i+charLen:]
									if isAnswerFieldEnd(tail) {
										inAnswer = false
										answerCompletelyStreamed = true
										advanceBytes = i + charLen
										break
									}
									// Not a real end — treat as embedded unescaped quote
									contentBuilder.WriteRune('"')
									advanceBytes = i + charLen
									continue
								}
								contentBuilder.WriteRune(char)
								advanceBytes = i + charLen
							}
							if !answerCompletelyStreamed && isEscaped {
								advanceBytes -= 1
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
							processed += advanceBytes
						}

						// Keep only unprocessed tail in pending buffer
						remaining := pendingStr[processed:]
						pending.Reset()
						pending.WriteString(remaining)
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

		// Answer quality check on streaming path (ADR-000604: hard stop for bad answers)
		qualityFlags := AssessAnswerQuality(
			parsedAnswer.Answer, input.Query, parsedAnswer.Citations, promptData.intentType,
		)
		if len(qualityFlags) > 0 {
			u.logger.Info("stream_answer_quality_flags",
				slog.String("retrieval_set_id", promptData.retrievalSetID),
				slog.Any("flags", qualityFlags))
		}

		if parsedAnswer.ShortAnswer {
			u.logger.Warn("stream_short_answer_detected",
				slog.String("retrieval_set_id", promptData.retrievalSetID),
				slog.Int("answer_rune_length", utf8.RuneCountInString(parsedAnswer.Answer)),
				slog.String("query", input.Query))

			// Hard stop: short answer + quality issues = fallback instead of serving bad content.
			// A short answer alone might be acceptable (yes/no questions), but combined with
			// low keyword coverage or incoherent ending, it indicates a broken generation.
			if hasQualityFlag(qualityFlags, "low_keyword_coverage") || hasQualityFlag(qualityFlags, "incoherent_ending") {
				u.logger.Warn("stream_short_answer_hard_stop",
					slog.String("retrieval_set_id", promptData.retrievalSetID),
					slog.Any("flags", qualityFlags),
					slog.String("raw_response_preview", truncate(rawResponse, 300)))
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindFallback,
					Payload: "answer quality insufficient: short answer with quality issues",
				})
				return
			}
		}

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
				RetrievalSetID:        promptData.retrievalSetID,
				PromptVersion:         u.promptVersion,
				ExpandedQueries:       promptData.expandedQueries,
				StrategyUsed:          promptData.strategyUsed,
				IntentType:            string(promptData.intentType),
				SubIntentType:         string(promptData.subIntentType),
				RetrievalQuality:      string(promptData.retrievalQuality),
				ToolsUsed:             promptData.toolsUsed,
				RetrievalPolicy:       promptData.retrievalPolicy,
				GeneralRetrievalGated: promptData.generalGated,
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

// isAnswerFieldEnd checks whether the text after a closing " indicates
// the real end of the JSON "answer" field. Ollama structured output may
// produce unescaped quotes inside string values (known bug with HTML/code
// content). A real field end is followed by , or } (with optional whitespace)
// which form valid JSON structure. If the tail is empty (quote at chunk
// boundary), we conservatively return false so the parser buffers until
// more data arrives.
// hasQualityFlag checks if a specific flag is present in the quality flags list.
func hasQualityFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if f == flag {
			return true
		}
	}
	return false
}

func isAnswerFieldEnd(tail string) bool {
	if tail == "" {
		return false // not enough data — buffer and wait for next chunk
	}
	for _, r := range tail {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		case ',', '}':
			return true
		default:
			return false // e.g. 'o' from og:title — embedded quote
		}
	}
	return false // only whitespace — need more data
}

func (u *answerWithRAGUsecase) sendStreamEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
