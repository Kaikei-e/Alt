package job

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"alt/shared/port/knowledge_event_port"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// outboxRepository abstracts the outbox capabilities the worker needs.
//
// Since ADR-000954 Wave 3 the implementation is
// datahub_gateway.OutboxGateway, which calls alt-data-hub over mutual TLS;
// the harvester no longer touches outbox_events itself. Three method-shaped
// consequences of that move are visible here:
//
//   - The claim is one call, because the SELECT ... FOR UPDATE SKIP LOCKED and
//     the UPDATE ... SET status='PROCESSING' are one transaction on the
//     provider. There is no "mark processing" for this worker to forget.
//   - Recording an outcome and releasing a claim are separate methods rather
//     than one method with a status string. They were the same call before,
//     which is how a release could be spelled as an outcome by accident.
//   - The event type is domain.OutboxEvent, not the driver's row struct, so
//     this file no longer imports the database driver at all.
type outboxRepository interface {
	ClaimBatch(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkProcessed(ctx context.Context, id string, status domain.OutboxEventStatus, errorMessage string) error
	Release(ctx context.Context, id string) error
}

// statusUpdateTimeout bounds a detached status-write: it must survive the
// parent job context being canceled (see processOutboxEvents), but a hung DB
// still shouldn't block the worker goroutine forever.
const statusUpdateTimeout = 10 * time.Second

// outboxClaimBatchSize is how many events one tick takes. The provider clamps
// it too; sending it explicitly keeps the batch size a property of this job,
// which is the thing that has to finish processing them inside its timeout.
const outboxClaimBatchSize = 10

// outboxWorkerTickInterval is this job's own schedule interval (registry.go
// registers it with Interval: outboxWorkerTickInterval). maxOutboxUpsertAttempts
// is defined in terms of it so the retry-budget reasoning below stays true if
// the schedule ever changes, instead of silently decoupling from it — which is
// exactly how the budget went stale the first time (see maxOutboxUpsertAttempts).
const outboxWorkerTickInterval = 5 * time.Second

// OutboxWorkerJob returns a function suitable for the JobScheduler that
// processes pending outbox events.
//
// repo is required. A nil one would make every tick claim nothing, which is
// indistinguishable from a drained outbox in the logs — the worker would look
// healthy while no article ever reached rag-orchestrator (CLAUDE.md rule 8).
func OutboxWorkerJob(repo outboxRepository, ragIntegration rag_integration_port.ArticleUpsertPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort) func(ctx context.Context) error {
	if repo == nil {
		panic("outbox-worker: outbox repository is nil — must be wired unconditionally at composition root (see .claude/rules/di-wiring.md)")
	}
	// retries lives here, not inside processOutboxEvents, so the attempt
	// count survives from one 5s tick to the next for the life of this
	// job's closure — the whole point of bounding retries at
	// maxOutboxUpsertAttempts rather than a single tick.
	retries := newOutboxRetryTracker()
	return func(ctx context.Context) error {
		return processOutboxEvents(ctx, repo, ragIntegration, knowledgeEventPort, retries)
	}
}

func processOutboxEvents(ctx context.Context, repo outboxRepository, ragIntegration rag_integration_port.ArticleUpsertPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort, retries *outboxRetryTracker) error {
	initOutboxMetrics()

	events, err := repo.ClaimBatch(ctx, outboxClaimBatchSize)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to claim pending outbox events", "error", err)
		return fmt.Errorf("claim pending outbox events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	logger.Logger.InfoContext(ctx, "Processing outbox events", "count", len(events))

	for i, event := range events {
		if ctx.Err() != nil {
			// The job timeout canceled ctx after processing already started.
			// events[i:] were claimed (status=PROCESSING) but never attempted;
			// release them back to PENDING so the next tick retries them
			// instead of leaving PROCESSING zombies that ClaimOutboxBatch
			// (PENDING-only) never re-fetches.
			logger.Logger.WarnContext(ctx, "outbox worker: context canceled mid-batch, releasing unattempted events to PENDING",
				"remaining", len(events)-i)
			resetClaimedEventsToPending(ctx, repo, events[i:])
			return nil
		}

		if event.EventType == "ARTICLE_UPSERT" {
			upsertInput, err := parseArticleUpsertPayload(event.Payload)
			if err != nil {
				if errors.Is(err, errMissingUserID) {
					logger.Logger.ErrorContext(ctx, "ARTICLE_UPSERT outbox event missing owner user_id",
						"event_id", event.ID, "article_id", upsertInput.ArticleID)
					markProcessed(ctx, repo, event.ID, domain.OutboxFailed, "missing owner user_id")
					outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "missing_user_id")))
					continue
				}
				// A malformed payload will never unmarshal differently on
				// retry — this is genuinely terminal, unlike the RAG call
				// below.
				logger.Logger.ErrorContext(ctx, "Failed to unmarshal outbox event payload", "event_id", event.ID, "error", err)
				markProcessed(ctx, repo, event.ID, domain.OutboxFailed, err.Error())
				outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "unmarshal_error")))
				continue
			}

			// This is the only call to RagIntegrationPort.UpsertArticle in the
			// codebase: the outbox is the sole delivery route into the RAG
			// index, not a reliability backstop for a separate direct-call
			// path.
			//
			// It is skipped for a row this process already upserted, which is
			// a row released only because its ArticleCreated append failed.
			// Repeating the upsert would re-run 10-30s of embedding for a
			// document already in the index, and it is exactly the rows in
			// that state that a sovereign outage re-claims every tick.
			if retries.ragUpsertDone(event.ID) {
				logger.Logger.InfoContext(ctx, "Skipping RAG upsert already delivered in an earlier tick, retrying ArticleCreated only",
					"event_id", event.ID)
			} else {
				if err := ragIntegration.UpsertArticle(ctx, upsertInput); err != nil {
					handleUpsertFailure(ctx, repo, retries, event.ID, err)
					// Knowledge Home must not wait on RAG success (ADR-000578):
					// still attempt ArticleCreated. Emit failure on this branch
					// cannot reopen a terminally FAILED row — orphan repair
					// covers that case. On a transient RAG release the next
					// tick retries both side effects (ArticleCreated is
					// dedupe-safe).
					if emitErr := emitArticleCreatedEvent(ctx, knowledgeEventPort, event.Payload); emitErr != nil {
						logger.Logger.ErrorContext(ctx, "ArticleCreated emit failed after RAG upsert failure",
							"event_id", event.ID, "error", emitErr)
					}
					continue
				}
				retries.markRagUpserted(event.ID)
			}

			// ACK only after both side effects are durable. Marking
			// PROCESSED before AppendKnowledgeEvent (the previous order)
			// left PROCESSED outbox rows with no ArticleCreated whenever
			// sovereign was briefly unavailable — Home rows then arrived
			// via SummaryVersionCreated with blank title/url and Trail
			// fell back to article:<uuid>.
			if err := emitArticleCreatedEvent(ctx, knowledgeEventPort, event.Payload); err != nil {
				handleArticleCreatedFailure(ctx, repo, retries, event.ID, err)
				continue
			}

			retries.clear(event.ID)
			logger.Logger.InfoContext(ctx, "Successfully processed outbox event", "event_id", event.ID)
			markProcessed(ctx, repo, event.ID, domain.OutboxProcessed, "")
			outboxProcessedCounter.Add(ctx, 1)
		} else {
			// An event type this worker was never taught to handle will
			// never become one it handles by retrying — terminal, same as
			// the unmarshal case above.
			logger.Logger.WarnContext(ctx, "Unknown event type", "event_type", event.EventType, "event_id", event.ID)
			markProcessed(ctx, repo, event.ID, domain.OutboxFailed, "Unknown event type")
			outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "unknown_event_type")))
		}
	}
	return nil
}

// handleUpsertFailure decides whether a failed UpsertArticle call gets
// another try or ends the row's lifecycle.
//
// Only errors augur_adapter marks with rag_integration_port.ErrRagUpsertTransient
// (a transport failure or a 5xx from rag-orchestrator) are retried — the
// exact class of error the evidence behind this fix showed going straight to
// FAILED with zero recourse (254 rows, all "RAG UpsertIndex returned non-OK
// status: 500", zero PROCESSED for six days). A 4xx, or any other
// RagIntegrationPort implementation returning a plain error, keeps the
// pre-fix terminal behavior: retrying a permanent rejection cannot succeed.
func handleUpsertFailure(ctx context.Context, repo outboxRepository, retries *outboxRetryTracker, eventID string, err error) {
	if errors.Is(err, rag_integration_port.ErrRagUpsertTransient) {
		attempt := retries.recordFailure(eventID)
		if !isRetryBudgetExhausted(attempt, maxOutboxUpsertAttempts) {
			logger.Logger.WarnContext(ctx, "Transient RAG upsert failure, releasing outbox event for retry",
				"event_id", eventID, "attempt", attempt, "max_attempts", maxOutboxUpsertAttempts, "error", err)
			if releaseForRetry(ctx, repo, eventID) {
				outboxRetriedCounter.Add(ctx, 1)
			} else {
				// The Release RPC itself failed: the row is still
				// PROCESSING, not PENDING. Counting it as "retried" would
				// claim the next tick will pick it back up when nothing
				// guarantees that — it is a zombie row until an operator
				// notices this counter and re-queues it by hand.
				outboxReleaseFailedCounter.Add(ctx, 1)
			}
			return
		}

		logger.Logger.ErrorContext(ctx, "RAG upsert exhausted retries, marking outbox event FAILED",
			"event_id", eventID, "attempts", attempt, "error", err)
		retries.clear(eventID)
		markProcessed(ctx, repo, eventID, domain.OutboxFailed, err.Error())
		outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "retries_exhausted")))
		return
	}

	logger.Logger.ErrorContext(ctx, "Failed to upsert article to RAG from outbox", "event_id", eventID, "error", err)
	retries.clear(eventID)
	markProcessed(ctx, repo, eventID, domain.OutboxFailed, err.Error())
	outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "non_retryable_error")))
}

// handleArticleCreatedFailure decides whether a row whose RAG upsert
// succeeded but whose ArticleCreated append did not gets another try.
//
// Every append failure is transient by construction — an invalid payload is
// skipped inside emitArticleCreatedEvent rather than returned — so unlike
// handleUpsertFailure there is no permanent class to sort out here. What it
// shares is the budget's size, not its remainder: a row whose upsert landed
// starts this count from zero (markRagUpserted). The release used to be
// unconditional, which made this the one delivery path with no ceiling at all.
// The claim is oldest-first LIMIT 10, so an unbounded release is not "this row
// waits" but "this row occupies a tenth of every tick and every newer article
// waits behind it", for as long as knowledge-sovereign is down.
func handleArticleCreatedFailure(ctx context.Context, repo outboxRepository, retries *outboxRetryTracker, eventID string, err error) {
	attempt := retries.recordFailure(eventID)
	if !isRetryBudgetExhausted(attempt, maxOutboxUpsertAttempts) {
		logger.Logger.WarnContext(ctx, "ArticleCreated emit failed after RAG success; releasing outbox event for retry",
			"event_id", eventID, "attempt", attempt, "max_attempts", maxOutboxUpsertAttempts, "error", err)
		if releaseForRetry(ctx, repo, eventID) {
			outboxRetriedCounter.Add(ctx, 1)
		} else {
			outboxReleaseFailedCounter.Add(ctx, 1)
		}
		return
	}

	logger.Logger.ErrorContext(ctx, "ArticleCreated emit exhausted retries, marking outbox event FAILED",
		"event_id", eventID, "attempts", attempt, "error", err)
	retries.clear(eventID)
	markProcessed(ctx, repo, eventID, domain.OutboxFailed, err.Error())
	outboxFailedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("reason", "article_created_retries_exhausted")))
}

// releaseForRetry returns a transiently-failed event to PENDING on a context
// detached from the caller's job context, for the same reason markProcessed
// detaches: the write is an RPC to alt-data-hub, and a canceled job timeout
// must not abort the release itself. The bool return tells the caller
// whether the release actually happened, so a failed RPC isn't counted the
// same as a real retry (see outboxReleaseFailedCounter).
func releaseForRetry(ctx context.Context, repo outboxRepository, id string) bool {
	detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusUpdateTimeout)
	defer cancel()
	if err := repo.Release(detachedCtx, id); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to release outbox event for retry", "event_id", id, "error", err)
		return false
	}
	return true
}

// emitArticleCreatedEvent appends a Knowledge Home ArticleCreated event to sovereign-db.
// Uses dedupe_key for idempotency — safe to call on every ARTICLE_UPSERT.
//
// port is a required composition-root dependency (job/registry.go always
// wires container.SovereignClient here). A nil port means DI forgot to wire
// the Knowledge Home event producer — panicking surfaces that immediately
// instead of silently dropping every ArticleCreated event (CLAUDE.md rule 8 /
// ADR-000928 root cause).
func emitArticleCreatedEvent(ctx context.Context, port knowledge_event_port.AppendKnowledgeEventPort, payload []byte) error {
	if port == nil {
		panic("outbox_worker: knowledge_event_port.AppendKnowledgeEventPort is nil — the Knowledge Home ArticleCreated producer must be wired at composition root (see .claude/rules/di-wiring.md)")
	}

	var probe struct {
		ArticleID string `json:"article_id"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(payload, &probe); err == nil && probe.UpdatedAt == "" {
		// Only reachable for outbox rows enqueued before this field existed.
		logger.Logger.WarnContext(ctx, "outbox payload missing updated_at, falling back to processing-time wall clock",
			"article_id", probe.ArticleID)
	}

	kevent, err := buildArticleCreatedKnowledgeEvent(payload, time.Now())
	if err != nil {
		if errors.Is(err, errSkipInvalidUserID) {
			userID := ""
			var target *errInvalidUserID
			if errors.As(err, &target) {
				userID = target.userID
			}
			logger.Logger.WarnContext(ctx, "invalid user_id for knowledge event, skipping", "user_id", userID)
			// Invalid user_id is a permanent payload defect: skipping (nil error)
			// lets the caller ACK rather than retry forever on the same bad row.
			return nil
		}
		var updatedErr *errInvalidUpdatedAt
		if errors.As(err, &updatedErr) {
			logger.Logger.ErrorContext(ctx, "outbox payload updated_at not RFC3339, withholding knowledge event",
				"article_id", updatedErr.articleID, "updated_at", updatedErr.updatedAt, "error", updatedErr.err)
			return err
		}
		var marshalErr *errMarshalKnowledgePayload
		if errors.As(err, &marshalErr) {
			logger.Logger.ErrorContext(ctx, "failed to marshal knowledge ArticleCreated payload, skipping",
				"article_id", marshalErr.articleID, "error", marshalErr.err)
			return err
		}
		logger.Logger.ErrorContext(ctx, "failed to unmarshal outbox payload for knowledge event", "error", err)
		return err
	}

	if _, err := port.AppendKnowledgeEvent(ctx, *kevent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append knowledge ArticleCreated event",
			"article_id", kevent.AggregateID, "error", err)
		return fmt.Errorf("append knowledge ArticleCreated event: %w", err)
	}
	return nil
}

// markProcessed writes the outbox event's terminal status on a context
// detached from the caller's job context.
//
// A job-timeout cancellation must not block the status write itself, or the
// row is left at the PROCESSING status the claim set, which the PENDING-only
// claim query never re-fetches — the production zombie-row incident this
// fixes. The detachment matters more now than it did: the write is an RPC to
// alt-data-hub, so a cancelled context aborts it at the transport before the
// provider ever sees it. ctx is still used for logging so a cancellation shows
// up in the right trace/log context.
func markProcessed(ctx context.Context, repo outboxRepository, id string, status domain.OutboxEventStatus, errMsg string) {
	detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusUpdateTimeout)
	defer cancel()
	if err := repo.MarkProcessed(detachedCtx, id, status, errMsg); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to update outbox event status", "event_id", id, "status", status, "error", err)
	}
}

// resetClaimedEventsToPending releases events that were claimed (locked to
// PROCESSING by ClaimOutboxBatch) but never attempted, back to PENDING, so the
// next tick retries them instead of leaving them stuck. ctx is the
// (already-canceled) job context, passed through only for log correlation —
// the write below detaches it.
func resetClaimedEventsToPending(ctx context.Context, repo outboxRepository, events []domain.OutboxEvent) {
	for _, event := range events {
		detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusUpdateTimeout)
		if err := repo.Release(detachedCtx, event.ID); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to release claimed outbox event", "event_id", event.ID, "error", err)
		}
		cancel()
	}
}
