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
//
// Invariant: every invocation emits exactly one terminal StreamEventKindDone
// event with a non-nil *AnswerWithRAGOutput payload as the LAST event before
// the channel is closed. The Done is fired by a deferred closure so that all
// return paths (success, fallback, error, clarification) honour the contract
// without each call site having to remember to emit it. Persistence on the
// handler side keys off Done.Answer != "" — empty answers signal "nothing
// worth keeping" (clarification, hard-fail before any LLM output).
func (u *answerWithRAGUsecase) Stream(ctx context.Context, input AnswerWithRAGInput) <-chan StreamEvent {
	events := make(chan StreamEvent, 4)
	go func() {
		defer close(events)

		// Single source of truth for the terminal Done event. Code paths mutate
		// this; the deferred emit below guarantees one — and only one — Done
		// reaches the handler.
		finalOutput := &AnswerWithRAGOutput{}
		defer func() {
			// Use a background context so a cancelled request context can never
			// silently drop the terminal event. The select keeps it non-blocking
			// in the unlikely case the buffered channel is already full.
			select {
			case events <- StreamEvent{Kind: StreamEventKindDone, Payload: finalOutput}:
			default:
			}
		}()

		if strings.TrimSpace(input.Query) == "" {
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindError,
				Payload: "query is required",
			})
			finalOutput.Reason = "query is required"
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
			finalOutput = val
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

		// 3. Prepare Context (query planning + retrieval + reranking)
		// Run buildPrompt in a goroutine with a heartbeat ticker to keep data
		// flowing through Cloudflare Tunnel. Without this, Cloudflare's 30-second
		// Proxy Write Timeout kills the connection during retrieval + reranking.
		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "planning",
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
					finalOutput.Fallback = true
					finalOutput.Reason = result.err.Error()
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
			// Clarification has no assistant answer to persist; deferred Done
			// fires with empty Answer so the handler skips persistence.
			finalOutput.Reason = "clarification requested"
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

		profile := deriveAcceptanceProfile(input, promptData)
		if profile.strictLongForm {
			// Hybrid streaming: use ChatStream with paragraph-level provisional
			// previews instead of non-streaming generateAnswerWithRetries.
			// Raw tokens are accumulated internally; flushes happen at paragraph
			// or sentence boundaries via ParagraphFlusher.
			//
			// The hybrid path returns its terminal *AnswerWithRAGOutput so we
			// can plumb it through this function's deferred Done emit. This
			// keeps the "exactly one Done" invariant intact across both paths.
			finalOutput = u.streamHybridLongForm(ctx, events, input, promptData, profile, heartbeat, cacheKey)
			return
		}

		// 4. Single Stage Generation (Streaming) — use messages from buildPrompt directly
		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "drafting",
		}) {
			return
		}
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
					reason := fmt.Sprintf("llm chat stream setup failed: %v", result.err)
					u.sendStreamEvent(ctx, events, StreamEvent{
						Kind:    StreamEventKindFallback,
						Payload: reason,
					})
					finalOutput.Fallback = true
					finalOutput.Reason = reason
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
		var answerBuilder strings.Builder
		var pending strings.Builder
		inAnswer := false
		isEscaped := false
		answerCompletelyStreamed := false
		hasData := false
		chunkStream := chunkCh
		errStream := errCh
		done := false

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

					// Incremental parsing keeps the answer boundary intact even when
					// Ollama emits unescaped quotes inside structured output.
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
								} else if len(pendingStr) > 20 {
									processed = len(pendingStr) - 20
								}
							} else if len(pendingStr) > 20 {
								processed = len(pendingStr) - 20
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
									tail := strToScan[i+charLen:]
									if isAnswerFieldEnd(tail) {
										inAnswer = false
										answerCompletelyStreamed = true
										advanceBytes = i + charLen
										break
									}
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
								answerBuilder.WriteString(strToStream)
							}
							processed += advanceBytes
						}

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
				reason := fmt.Sprintf("llm stream failed: %v", streamErr)
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindFallback,
					Payload: reason,
				})
				finalOutput.Answer = strings.TrimSpace(answerBuilder.String())
				finalOutput.Fallback = true
				finalOutput.Reason = reason
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
			slog.Bool("answerCompletelyStreamed", true))

		if !hasData {
			u.logger.Warn("stream_no_data_fallback",
				slog.String("retrieval_set_id", promptData.retrievalSetID))
			const reason = "llm stream produced no data"
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: reason,
			})
			finalOutput.Fallback = true
			finalOutput.Reason = reason
			return
		}

		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "validating",
		})
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
			reason := fmt.Sprintf("validation failed: %v", err)
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: reason,
			})
			finalOutput.Answer = strings.TrimSpace(answerBuilder.String())
			finalOutput.Fallback = true
			finalOutput.Reason = reason
			return
		}

		u.logger.Info("stream_validation_completed",
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.Bool("fallback", parsedAnswer.Fallback),
			slog.String("reason", parsedAnswer.Reason),
			slog.Int("answer_length", len(parsedAnswer.Answer)),
			slog.Int("citations_count", len(parsedAnswer.Citations)))

		qualityFlags := AssessAnswerQuality(
			parsedAnswer.Answer, input.Query, parsedAnswer.Citations, promptData.intentType, promptData.expandedQueries,
		)
		if len(qualityFlags) > 0 {
			u.logger.Info("stream_answer_quality_flags",
				slog.String("retrieval_set_id", promptData.retrievalSetID),
				slog.Any("flags", qualityFlags))
		}
		finalAnswerText := parsedAnswer.Answer
		if answerBuilder.Len() > 0 {
			finalAnswerText = strings.TrimSpace(answerBuilder.String())
			parsedAnswer.Answer = finalAnswerText
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
			finalOutput.Answer = strings.TrimSpace(answerBuilder.String())
			finalOutput.Fallback = true
			finalOutput.Reason = parsedAnswer.Reason
			return
		}

		finalPromptData, finalAnswer, finalFlags, retryCount, accepted, err := u.retryValidatedAnswer(
			ctx, input, promptData, parsedAnswer, qualityFlags, profile, promptData.retrievalSetID, 0,
		)
		if err != nil {
			reason := fmt.Sprintf("generation failed: %v", err)
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: reason,
			})
			finalOutput.Answer = strings.TrimSpace(answerBuilder.String())
			finalOutput.Fallback = true
			finalOutput.Reason = reason
			return
		}
		if !accepted {
			fallbackReason := selectFallbackReason(finalPromptData.intentType, finalFlags)
			if finalAnswer != nil && finalAnswer.Fallback {
				fallbackReason = finalAnswer.Reason
			}
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fallbackReason,
			})
			finalOutput.Answer = strings.TrimSpace(answerBuilder.String())
			finalOutput.Fallback = true
			finalOutput.Reason = fallbackReason
			return
		}
		promptData = finalPromptData
		parsedAnswer = finalAnswer
		qualityFlags = finalFlags
		finalAnswerText = strings.TrimSpace(parsedAnswer.Answer)

		// Build Final Output (Hydration)
		finalCitations := u.buildCitations(promptData.contexts, parsedAnswer.Citations)

		output := &AnswerWithRAGOutput{
			Answer:    finalAnswerText,
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
				RetryCount:            promptData.retryCount + retryCount,
				ToolsUsed:             promptData.toolsUsed,
				RetrievalPolicy:       promptData.retrievalPolicy,
				GeneralRetrievalGated: promptData.generalGated,
			},
		}

		if !u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindDelta,
			Payload: output.Answer,
		}) {
			return
		}

		// Persist conversation state for follow-up reference resolution.
		if u.conversationStore != nil && input.UserID != "" {
			newState := DeriveStateUpdate(
				u.conversationStore.Get(input.UserID),
				input.UserID,
				promptData.parsedIntent,
				promptData.plannerOutput,
				output,
			)
			u.conversationStore.Put(newState)
		}

		// Store in Cache
		u.cache.Add(cacheKey, output)

		finalOutput = output
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
// selectFallbackReason chooses a specific fallback reason based on intent type and quality flags.
func selectFallbackReason(intentType IntentType, flags []string) string {
	if intentType == IntentCausalExplanation {
		return "十分に一貫した根拠が取れなかったため、因果関係を断定できません。より具体的な質問をお試しください。"
	}
	if hasQualityFlag(flags, "low_keyword_coverage") {
		return "answer quality insufficient: low keyword coverage"
	}
	return "answer quality insufficient: short answer with quality issues"
}

func shouldHardStopShortAnswer(intentType IntentType, flags []string) bool {
	return hasQualityFlag(flags, "low_keyword_coverage") ||
		hasQualityFlag(flags, "incoherent_ending") ||
		hasQualityFlag(flags, "context_insufficiency_disclaimer") ||
		(hasQualityFlag(flags, "expansion_failed") && intentType == IntentCausalExplanation)
}

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
