package job

import (
	"sync"
)

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

// isRetryBudgetExhausted reports whether the attempt count has reached or exceeded maxAttempts.
func isRetryBudgetExhausted(attempt int, maxAttempts int) bool {
	return attempt >= maxAttempts
}

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
