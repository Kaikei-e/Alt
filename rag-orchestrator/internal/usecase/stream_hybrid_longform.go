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
//
// streamHybridLongForm runs the long-form ChatStream path. It returns a
// non-nil *AnswerWithRAGOutput so the caller (Stream) can forward it to its
// deferred Done emit. Empty Answer means "nothing worth persisting"
// (clarification, hard-fail before any LLM output, ctx cancellation).
//
// Invariant: every return path leaves a non-nil pointer. Fallback returns set
// Fallback=true and Reason; success returns the full output.
func (u *answerWithRAGUsecase) streamHybridLongForm(
	ctx context.Context,
	events chan<- StreamEvent,
	input AnswerWithRAGInput,
	promptData *promptBuildResult,
	profile answerAcceptanceProfile,
	heartbeat *time.Ticker,
	cacheKey string,
) *AnswerWithRAGOutput {
	out := &AnswerWithRAGOutput{}
	if !u.sendStreamEvent(ctx, events, StreamEvent{
		Kind:    StreamEventKindProgress,
		Payload: "drafting",
	}) {
		return out
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
			return out
		case result := <-chatStreamCh:
			if result.err != nil {
				reason := fmt.Sprintf("llm chat stream setup failed: %v", result.err)
				u.sendStreamEvent(ctx, events, StreamEvent{
					Kind:    StreamEventKindFallback,
					Payload: reason,
				})
				out.Fallback = true
				out.Reason = reason
				return out
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
	answerParser := &incrementalAnswerParser{}
	hasData := false
	done := false

	// Flush smaller provisional previews so the UI feels live sooner.
	flusher := NewParagraphFlusher(40, 1500*time.Millisecond, 40)
	flushTicker := time.NewTicker(500 * time.Millisecond)
	defer flushTicker.Stop()

	chunkStream := chunkCh
	errStream := errCh

	for chunkStream != nil || errStream != nil {
		select {
		case <-ctx.Done():
			drainLLMStream(chunkStream, errStream)
			return out
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

				if strToStream := answerParser.Feed(chunk.Response); strToStream != "" {
					answerBuilder.WriteString(strToStream)
					// Feed to flusher for paragraph-level preview
					if flush, ok := flusher.Feed(strToStream); ok {
						u.sendStreamEvent(ctx, events, StreamEvent{
							Kind:    StreamEventKindDelta,
							Payload: flush,
						})
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
			reason := fmt.Sprintf("llm stream failed: %v", streamErr)
			u.sendStreamEvent(ctx, events, StreamEvent{
				Kind:    StreamEventKindFallback,
				Payload: reason,
			})
			out.Answer = strings.TrimSpace(answerBuilder.String())
			out.Fallback = true
			out.Reason = reason
			return out
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
		const reason = "llm stream produced no data"
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: reason,
		})
		out.Fallback = true
		out.Reason = reason
		return out
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
		reason := fmt.Sprintf("validation failed: %v", err)
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: reason,
		})
		out.Answer = strings.TrimSpace(answerBuilder.String())
		out.Fallback = true
		out.Reason = reason
		return out
	}

	if parsedAnswer.Fallback {
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: parsedAnswer.Reason,
		})
		out.Answer = strings.TrimSpace(answerBuilder.String())
		out.Fallback = true
		out.Reason = parsedAnswer.Reason
		return out
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
		reason := fmt.Sprintf("generation failed: %v", retryErr)
		u.sendStreamEvent(ctx, events, StreamEvent{
			Kind:    StreamEventKindFallback,
			Payload: reason,
		})
		out.Answer = strings.TrimSpace(answerBuilder.String())
		out.Fallback = true
		out.Reason = reason
		return out
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
		out.Answer = strings.TrimSpace(answerBuilder.String())
		out.Fallback = true
		out.Reason = fallbackReason
		return out
	}

	promptData = finalPromptData
	parsedAnswer = finalAnswer
	qualityFlags = finalFlags
	finalAnswerText = strings.TrimSpace(parsedAnswer.Answer)

	// Build final output
	finalCitations := u.buildCitations(promptData.contexts, parsedAnswer.Citations)
	relatedCitations := u.buildRelatedCitations(ctx, finalCitations, input.Query)
	output := &AnswerWithRAGOutput{
		Answer:           finalAnswerText,
		Citations:        finalCitations,
		RelatedCitations: relatedCitations,
		Contexts:         promptData.contexts,
		Fallback:         false,
		Reason:           "",
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
	if threadKey := conversationThreadKey(input); u.conversationStore != nil && threadKey != "" {
		newState := DeriveStateUpdate(
			u.conversationStore.Get(threadKey),
			threadKey,
			promptData.parsedIntent,
			promptData.plannerOutput,
			output,
		)
		u.conversationStore.Put(newState)
	}
	u.cache.Add(cacheKey, output)

	// The caller (Stream) emits the terminal Done via its deferred closure;
	// returning the output is enough to surface the authoritative answer.
	return output
}
