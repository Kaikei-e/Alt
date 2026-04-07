package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
)

// streamHybridLongForm implements the hybrid streaming path for detail/synthesis
// intents. It uses ChatStream for real-time token consumption, ParagraphFlusher
// for paragraph-level provisional preview deltas, and retains corrective retry
// with the "refining" progress stage.
//
// Flow:
//  1. Send progress=drafting
//  2. ChatStream → incremental answer parser → ParagraphFlusher → provisional delta events
//  3. Validate raw response
//  4. If retry needed: send progress=refining, run retry (no new deltas)
//  5. done.answer = authoritative final text (replaces all previews)
func (u *answerWithRAGUsecase) streamHybridLongForm(
	ctx context.Context,
	events chan<- StreamEvent,
	input AnswerWithRAGInput,
	promptData *promptBuildResult,
	profile answerAcceptanceProfile,
	heartbeat *time.Ticker,
	cacheKey string,
) {
	if !u.sendStreamEvent(ctx, events, StreamEvent{
		Kind:    StreamEventKindProgress,
		Payload: "drafting",
	}) {
		return
	}

	messages := promptData.messages

	// Start ChatStream in goroutine with heartbeat
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
waitHybridChatStream:
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
			break waitHybridChatStream
		case <-heartbeat.C:
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindHeartbeat,
				Payload: "",
			})
		}
	}

	// Stream with incremental answer parsing + paragraph flushing
	var builder strings.Builder
	var answerBuilder strings.Builder
	var pending strings.Builder
	inAnswer := false
	isEscaped := false
	answerCompletelyStreamed := false
	hasData := false
	done := false

	// Flush smaller provisional previews so the UI feels live sooner.
	flusher := NewParagraphFlusher(80, 1500*time.Millisecond, 80)
	flushTicker := time.NewTicker(500 * time.Millisecond)
	defer flushTicker.Stop()

	chunkStream := chunkCh
	errStream := errCh

	for chunkStream != nil || errStream != nil {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-chunkStream:
			if !ok {
				chunkStream = nil
				continue
			}
			if chunk.Thinking != "" {
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindThinking,
					Payload: chunk.Thinking,
				})
			}
			if chunk.Response != "" {
				hasData = true
				builder.WriteString(chunk.Response)

				if !answerCompletelyStreamed {
					pending.WriteString(chunk.Response)
					pendingStr := pending.String()
					processed := 0

					if !inAnswer {
						idx := strings.Index(pendingStr, "\"answer\"")
						if idx != -1 {
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
							// Feed to flusher for paragraph-level preview
							if flush, ok := flusher.Feed(strToStream); ok {
								u.sendStreamEvent(ctx, events, StreamEvent{
									Kind:    StreamEventKindDelta,
									Payload: flush,
								})
							}
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
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: fmt.Sprintf("llm stream failed: %v", streamErr),
			})
			return
		case <-flushTicker.C:
			// Time-based flush for pending content
			if flush, ok := flusher.TimeFlush(); ok {
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindDelta,
					Payload: flush,
				})
			}
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

	// Drain any remaining flusher content as final provisional delta
	if remaining := flusher.Drain(); remaining != "" {
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindDelta,
			Payload: remaining,
		})
	}

	if !hasData {
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: "llm stream produced no data",
		})
		return
	}

	// Validation
	u.sendStreamEvent(ctx, events, StreamEvent{
		Kind:    StreamEventKindProgress,
		Payload: "validating",
	})

	rawResponse := builder.String()
	u.logger.Info("hybrid_stream_validation_starting",
		slog.String("retrieval_set_id", promptData.retrievalSetID),
		slog.Int("raw_response_length", len(rawResponse)),
		slog.String("raw_response_preview", truncate(rawResponse, 500)))

	parsedAnswer, err := u.validator.Validate(rawResponse, promptData.contexts)
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

	qualityFlags := AssessAnswerQuality(
		parsedAnswer.Answer, input.Query, parsedAnswer.Citations, promptData.intentType, promptData.expandedQueries,
	)

	finalAnswerText := parsedAnswer.Answer
	if answerBuilder.Len() > 0 {
		finalAnswerText = strings.TrimSpace(answerBuilder.String())
		parsedAnswer.Answer = finalAnswerText
	}

	willRetry := profile.maxRetries > 0 &&
		u.shouldRetryGeneratedAnswer(input.Query, parsedAnswer, promptData, qualityFlags, profile)
	if willRetry {
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindThinking,
			Payload: "Refining explanation...",
		})
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindProgress,
			Payload: "refining",
		})
	}

	// Corrective retry if quality is insufficient
	finalPromptData, finalAnswer, finalFlags, retryCount, accepted, retryErr := u.retryValidatedAnswer(
		ctx, input, promptData, parsedAnswer, qualityFlags, profile, promptData.retrievalSetID, 0,
	)
	if retryErr != nil {
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: fmt.Sprintf("generation failed: %v", retryErr),
		})
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
		return
	}

	promptData = finalPromptData
	parsedAnswer = finalAnswer
	qualityFlags = finalFlags
	finalAnswerText = strings.TrimSpace(parsedAnswer.Answer)

	// Build final output
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
	if promptData.plannerOutput != nil {
		output.Debug.PlannerOperation = string(promptData.plannerOutput.Operation)
		output.Debug.PlannerConfidence = promptData.plannerOutput.Confidence
		output.Debug.NeedsClarification = promptData.plannerOutput.NeedsClarification
	}

	// Persist conversation state
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
	u.cache.Add(cacheKey, output)

	// Send authoritative done (replaces all provisional previews)
	u.sendStreamEvent(ctx, events, StreamEvent{
		Kind:    StreamEventKindDone,
		Payload: output,
	})
}
