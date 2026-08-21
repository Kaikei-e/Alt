package fetch_article_gateway

import (
	"context"
	"testing"
	"time"

	"alt/utils/rate_limiter"
)

// The prefetch class is defined by where the host's turn is taken, not by how
// long it is waited for. Its caller — the BatchPrefetchArticleContent handler
// — takes the turn itself, before it asks the scraping-policy gate, because
// losing the turn costs nothing while asking the gate reserves the publisher's
// crawl-delay window. A gateway that took the turn a second time would either
// stall against the slot it already holds or, worse, take a second one.
//
// So this gateway must carry no limiter of its own. That is the invariant the
// class is; if it ever regains one, the ordering above silently stops holding.
func TestPrefetchGatewayDoesNotTakeTheHostTurnItself(t *testing.T) {
	gw := NewPrefetchFetchArticleGateway()

	if gw.rateLimiter != nil {
		t.Fatal("the prefetch gateway must not hold a rate limiter: its caller has already taken the host's turn")
	}
	if gw.slotWaitBudget != 0 {
		t.Fatalf("the prefetch gateway has no turn to wait for, got budget %v", gw.slotWaitBudget)
	}
	if gw.ssrfValidator == nil {
		t.Fatal("the prefetch gateway must keep the SSRF validator; a warm reaches the open internet like any other fetch")
	}
	if gw.httpClient == nil {
		t.Fatal("the prefetch gateway must have a client")
	}
}

// waitForHostSlot is the method that would double-take the turn. With no
// limiter it must be a no-op rather than an error, since the caller's turn is
// the real one.
func TestPrefetchGatewayHostSlotWaitIsANoop(t *testing.T) {
	gw := NewPrefetchFetchArticleGateway()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := gw.waitForHostSlot(ctx, "https://example.com/article"); err != nil {
		t.Fatalf("expected no wait, got %v", err)
	}
}

// The interactive class is unchanged by the new one: it still holds the shared
// limiter and its own budget.
func TestInteractiveGatewayStillHoldsTheLimiter(t *testing.T) {
	limiter := rate_limiter.NewHostRateLimiter(10*time.Second, 3)
	gw := NewInteractiveFetchArticleGateway(limiter, nil, 2*time.Second)

	if gw.rateLimiter == nil {
		t.Fatal("the interactive gateway must keep taking the host's turn itself")
	}
	if gw.slotWaitBudget != 2*time.Second {
		t.Fatalf("slotWaitBudget = %v, want 2s", gw.slotWaitBudget)
	}
}

// A warm reaches this gateway's concurrency semaphore only after the
// scraping-policy gate has already granted — and a grant reserves the
// publisher's crawl-delay window. Queueing here would therefore hold a
// reserved window while waiting, and running out of budget in the queue would
// spend it on nothing. The pool upstream is the real bound; this must not add
// a narrower one.
func TestPrefetchGatewayDoesNotQueueBehindItself(t *testing.T) {
	gw := NewPrefetchFetchArticleGateway()

	if got, want := cap(gw.fetchSem), 8; got != want {
		t.Fatalf("fetchSem capacity = %d, want %d (the handler's warm pool size)", got, want)
	}
}
