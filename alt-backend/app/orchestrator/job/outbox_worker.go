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
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
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

// maxOutboxUpsertAttempts bounds how many times a row is released back to
// PENDING before it is given up as terminally FAILED. Both of the row's side
// effects draw on it — a transient RAG upsert failure (see
// rag_integration_port.ErrRagUpsertTransient) and a failed ArticleCreated
// append — because what it rations is claim slots, and a row occupies one
// whichever leg sent it back. Delivering one of them refreshes it (see
// markRagUpserted): the two legs talk to two different services, and a budget
// sized to outlast one service's redeploy is not a budget they can split.
//
// There is no attempt_count column on outbox_events, so this count lives in
// process memory (outboxRetryTracker) and resets on every harvester restart.
// The budget is sized in ticks of outboxWorkerTickInterval:
// (attempts-1) * outboxWorkerTickInterval is the minimum downtime a row
// survives before going terminal. A first cut at 3 attempts covered only
// ~10s of a 5s-interval job — far short of a real redeploy (observed 20-60s)
// — and reproduced the exact incident this fix exists to prevent for any
// outage longer than a few seconds. 24 attempts covers >=115s, comfortably
// above the observed 60s ceiling with margin for tick jitter under batch
// backlog.
//
// This is still bounded, not unbounded: a sustained outage (rag-orchestrator
// crash-looping, a multi-day incident) exhausts it exactly like the old
// budget did and frees the row instead of occupying the front of the
// oldest-first claim query forever. Recovering that case is an operational
// decision (re-queue the FAILED rows), not something this worker should do
// unbounded and unattended.
const maxOutboxUpsertAttempts = 24

// outboxRetryTracker counts consecutive delivery failures per outbox row,
// across worker ticks, in process memory only. It also remembers which rows
// already got their RAG upsert in, so a row released for the sake of its
// second side effect does not pay for the first one twice.
type outboxRetryTracker struct {
	mu          sync.Mutex
	attempts    map[string]int
	ragUpserted map[string]bool
}

func newOutboxRetryTracker() *outboxRetryTracker {
	return &outboxRetryTracker{
		attempts:    make(map[string]int),
		ragUpserted: make(map[string]bool),
	}
}

// recordFailure increments and returns the attempt count for id.
func (t *outboxRetryTracker) recordFailure(id string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attempts[id]++
	return t.attempts[id]
}

// markRagUpserted records that id's article reached the RAG index, and returns
// the attempt budget to full.
//
// Reaching the index is progress, and the budget is sized to outlast one
// downstream service being redeployed (see maxOutboxUpsertAttempts). The row's
// two legs talk to two different services, so carrying a budget spent on a
// rag-orchestrator outage over to the ArticleCreated leg hands that leg a
// window far shorter than the one it was sized for — at worst a single attempt
// — and ends the row FAILED on the tick both side effects were finally making
// progress. ragUpserted deliberately survives: the upsert must not be re-run.
func (t *outboxRetryTracker) markRagUpserted(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ragUpserted[id] = true
	delete(t.attempts, id)
}

// ragUpsertDone reports whether this process already delivered id's article to
// the RAG index. Only ever false-negative: a restart forgets, and the row is
// upserted again — which is safe, the upsert is idempotent on article_id, and
// costs one embedding run rather than a missed one.
func (t *outboxRetryTracker) ragUpsertDone(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ragUpserted[id]
}

// clear forgets id. Called once a row reaches a terminal status (PROCESSED or
// FAILED) so a long-running harvester process does not grow these maps by one
// entry per outbox row for the life of the process.
func (t *outboxRetryTracker) clear(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, id)
	delete(t.ragUpserted, id)
}

// These OTel counters are this file's fix for a specific gap: before this
// change nothing surfaced a stalled outbox — 5xx failures were marked FAILED
// with no metric, no alert and no health degradation, and the only way to
// notice was to query outbox_events directly. A sustained non-zero rate on
// the failed counter (reason "retries_exhausted" in particular) is the
// signal an alert should watch.
var (
	outboxMeterOnce            sync.Once
	outboxProcessedCounter     metric.Int64Counter
	outboxRetriedCounter       metric.Int64Counter
	outboxFailedCounter        metric.Int64Counter
	outboxReleaseFailedCounter metric.Int64Counter
)

func initOutboxMetrics() {
	outboxMeterOnce.Do(func() {
		meter := otel.Meter("alt-harvester.outbox-worker")
		outboxProcessedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_processed_total",
			metric.WithDescription("ARTICLE_UPSERT outbox events successfully delivered to RAG and knowledge-sovereign ArticleCreated"))
		outboxRetriedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_retried_total",
			metric.WithDescription("Outbox events released back to PENDING after a transient RAG upsert or ArticleCreated append failure"))
		outboxFailedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_failed_total",
			metric.WithDescription("Outbox events marked terminally FAILED, labeled by reason"))
		outboxReleaseFailedCounter, _ = meter.Int64Counter("alt_harvester_outbox_events_release_failed_total",
			metric.WithDescription("Outbox events where the release-to-PENDING RPC itself failed after a transient upsert failure, leaving the row stuck PROCESSING — invisible to both the PENDING claim query and a FAILED-status audit"))
	})
}

// OutboxWorkerJob returns a function suitable for the JobScheduler that
// processes pending outbox events.
//
// repo is required. A nil one would make every tick claim nothing, which is
// indistinguishable from a drained outbox in the logs — the worker would look
// healthy while no article ever reached rag-orchestrator (CLAUDE.md rule 8).
func OutboxWorkerJob(repo outboxRepository, ragIntegration rag_integration_port.RagIntegrationPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort) func(ctx context.Context) error {
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

func processOutboxEvents(ctx context.Context, repo outboxRepository, ragIntegration rag_integration_port.RagIntegrationPort, knowledgeEventPort knowledge_event_port.AppendKnowledgeEventPort, retries *outboxRetryTracker) error {
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
			var upsertInput rag_integration_port.UpsertArticleInput
			if err := json.Unmarshal(event.Payload, &upsertInput); err != nil {
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
		if attempt < maxOutboxUpsertAttempts {
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
	if attempt < maxOutboxUpsertAttempts {
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

	var p struct {
		ArticleID string `json:"article_id"`
		URL       string `json:"url"`
		Title     string `json:"title"`
		UserID    string `json:"user_id"`
		// UpdatedAt is stamped once at outbox-enqueue time (save_article_driver.go),
		// i.e. when the article-upsert fact actually occurred. Reused below as
		// PublishedAt instead of re-stamping wall-clock time here: this handler
		// can run at an arbitrary, possibly much later time (worker poll delay,
		// crash-and-reprocess), so reading time.Now() here would make the same
		// event replay to a different PublishedAt each time it's processed.
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to unmarshal outbox payload for knowledge event", "error", err)
		return fmt.Errorf("unmarshal outbox payload for knowledge event: %w", err)
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		logger.Logger.WarnContext(ctx, "invalid user_id for knowledge event, skipping", "user_id", p.UserID)
		// Invalid user_id is a permanent payload defect: skipping (nil error)
		// lets the caller ACK rather than retry forever on the same bad row.
		return nil
	}

	publishedAt := p.UpdatedAt
	if publishedAt == "" {
		// Only reachable for outbox rows enqueued before this field existed.
		logger.Logger.WarnContext(ctx, "outbox payload missing updated_at, falling back to processing-time wall clock",
			"article_id", p.ArticleID)
		publishedAt = time.Now().Format(time.RFC3339)
	}

	// occurred_at is the article-upsert fact's own timestamp, minted once at
	// outbox-enqueue time (same source as published_at above). Re-stamping
	// time.Now() here made the same event replay to a different occurred_at on
	// every reprocess (worker poll delay, crash-and-reprocess), breaking the
	// reproject-safe / no-business-fact-time.Now() invariant. Deriving it from
	// updated_at keeps the append idempotent under the article-scoped dedupe_key.
	occurredAt, occurredErr := time.Parse(time.RFC3339, publishedAt)
	if occurredErr != nil {
		// publishedAt is always RFC3339 (p.UpdatedAt from save_article_driver, or
		// the wall-clock fallback above), so this is defensive. Surface it as a
		// retryable failure rather than fabricating a fresh occurred_at.
		logger.Logger.ErrorContext(ctx, "outbox payload updated_at not RFC3339, withholding knowledge event",
			"article_id", p.ArticleID, "updated_at", publishedAt, "error", occurredErr)
		return fmt.Errorf("parse outbox updated_at for occurred_at: %w", occurredErr)
	}

	// Marshal through the canonical domain.ArticleCreatedPayload struct so
	// the wire key for the article URL is locked to "url" — using a raw
	// map[string]any literal here historically wrote the legacy "link" key
	// which silently broke the projector (PM-2026-041). The shared struct
	// is the single source of truth for this wire schema.
	eventPayload, err := json.Marshal(domain.ArticleCreatedPayload{
		ArticleID:   p.ArticleID,
		Title:       p.Title,
		PublishedAt: publishedAt,
		TenantID:    p.UserID,
		URL:         p.URL,
	})
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to marshal knowledge ArticleCreated payload, skipping",
			"article_id", p.ArticleID, "error", err)
		return fmt.Errorf("marshal knowledge ArticleCreated payload: %w", err)
	}

	kevent := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      userID,
		UserID:        &userID,
		ActorType:     domain.ActorService,
		ActorID:       "outbox-worker",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   p.ArticleID,
		DedupeKey:     fmt.Sprintf(domain.DedupeKeyArticleCreated, p.ArticleID),
		Payload:       eventPayload,
	}

	if _, err := port.AppendKnowledgeEvent(ctx, kevent); err != nil {
		logger.Logger.ErrorContext(ctx, "failed to append knowledge ArticleCreated event",
			"article_id", p.ArticleID, "error", err)
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
