// Package push_dispatch_usecase drains push_deliveries and sends each row to
// its device's push service.
//
// The one structural rule worth stating up front: the claim transaction is
// already committed before anything here runs. `FOR UPDATE SKIP LOCKED` gives a
// free claim only when the work happens inside the transaction, and the work
// here is an HTTPS request to Google, Mozilla or Apple. Holding a transaction
// across a third party's p99 would pin the MVCC horizon of a database shared by
// twenty other services. So the shape is: claim and commit, send with no
// transaction open, then finalize.
package push_dispatch_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"alt/domain"
	"alt/orchestrator/port/push_dispatch_port"
)

const (
	// maxAttempts is deliberately small. A general job queue retries for days
	// because eventual completion still has value; a "your recap is ready"
	// notification delivered days later is worse than one never delivered, and
	// the row's expires_at will have passed anyway.
	maxAttempts = 8

	backoffBase = 10 * time.Second
	backoffCap  = time.Hour
)

// Stats is one batch's outcome, for metrics and for the log line.
//
// Gone is counted apart from Dead on purpose: a user uninstalling the app is
// not a pipeline failure, and folding the two together destroys the ability to
// tell "our delivery is broken" from "some devices went away" — which is the
// distinction the delivery SLI is made of.
type Stats struct {
	Claimed  int
	Sent     int
	Released int
	Dead     int
	Gone     int
	// Attempts records what each send actually returned.
	//
	// Without the status code the alert that matters most here has nothing to
	// watch: a VAPID JWT signed once at startup works for a day and then makes
	// every push 401 forever, with no crash, no restart and no error rate —
	// the counters above would all read zero and look like an idle queue.
	Attempts []Attempt
}

// Attempt is one send's outcome, carrying both labels the delivery metric needs.
type Attempt struct {
	StatusCode int
	// Result is the SLI bucket: sent | gone | released | dead. `gone` is kept
	// out of the error budget — a user uninstalling is not our failure.
	Result string
}

type Usecase struct {
	deliveries    push_dispatch_port.DeliveryPort
	subscriptions push_dispatch_port.SubscriptionPort
	sender        push_dispatch_port.SenderPort

	// backoff is a field so tests can pin the schedule; production uses full
	// jitter, which spends less total work than plain exponential backoff
	// because it removes the clusters that synchronised retries create.
	backoff func(attempt int) time.Duration
}

func New(
	deliveries push_dispatch_port.DeliveryPort,
	subscriptions push_dispatch_port.SubscriptionPort,
	sender push_dispatch_port.SenderPort,
) *Usecase {
	// Rule 8: an unwired dependency here would present as "no notifications",
	// which is indistinguishable from an empty queue. Refuse to start instead.
	if deliveries == nil {
		panic("push dispatcher: delivery port is nil — must be wired at the composition root (see .claude/rules/di-wiring.md)")
	}
	if subscriptions == nil {
		panic("push dispatcher: subscription port is nil — the 410 branch would silently keep dead devices in the fan-out")
	}
	if sender == nil {
		panic("push dispatcher: sender is nil — the queue would drain into nothing")
	}

	return &Usecase{
		deliveries:    deliveries,
		subscriptions: subscriptions,
		sender:        sender,
		backoff:       fullJitterBackoff,
	}
}

func fullJitterBackoff(attempt int) time.Duration {
	window := backoffBase << min(attempt, 12)
	if window > backoffCap {
		window = backoffCap
	}
	return time.Duration(rand.Int64N(int64(window) + 1))
}

// DispatchBatch claims up to limit deliveries and settles each one.
//
// A failure against one device never aborts the batch: devices fail
// independently, and one unreachable endpoint must not stall the queue for
// everyone else. A failure to *claim*, on the other hand, propagates — an
// unreachable data plane looks exactly like an empty queue otherwise.
func (u *Usecase) DispatchBatch(ctx context.Context, limit int) (Stats, error) {
	var stats Stats

	batch, err := u.deliveries.ClaimBatch(ctx, limit)
	if err != nil {
		return stats, fmt.Errorf("claim push deliveries: %w", err)
	}
	stats.Claimed = len(batch)

	for _, delivery := range batch {
		u.settle(ctx, delivery, &stats)
	}

	return stats, nil
}

// deliveryPolicy is the per-kind product decision about how a notification
// travels: how long it stays worth delivering, whether it may wait for power or
// Wi-Fi, and what it collapses against.
//
// Topic is the kind itself. It is deliberately a constant rather than anything
// derived from the user or the artifact: RFC 8030 forwards it to no one but
// makes it visible to the push service in the clear, so a per-user value would
// hand Google, Mozilla and Apple a correlation key for free. Using the kind
// also means a second undelivered notification of the same kind replaces the
// first — five finished summaries become one buzz, and yesterday's unsent
// digest never arrives alongside today's.
func deliveryPolicy(kind string) (ttl time.Duration, urgency string, topic string) {
	if kind == domain.NotificationKindTodayEntranceReady {
		// Dies before the next morning's digest, so a digest delayed by an
		// outage can never surface a day late claiming to be today's.
		return 12 * time.Hour, "low", kind
	}
	// A finished job stays worth knowing about for a day; past that the next
	// one supersedes it anyway.
	return 24 * time.Hour, "", kind
}

func (u *Usecase) settle(ctx context.Context, delivery domain.PushDelivery, stats *Stats) {
	body, err := renderNotification(delivery.Kind, delivery.Payload)
	if err != nil {
		// renderNotification degrades rather than failing, so this is a
		// programming error rather than bad input. Refusing to send is the
		// right call: a push with no displayable content costs the site its
		// notification permission on Safari.
		u.finalize(ctx, delivery, 0, "payload render failed")
		stats.Dead++
		return
	}

	ttl, urgency, topic := deliveryPolicy(delivery.Kind)

	outcome, sendErr := u.sender.Send(ctx, push_dispatch_port.SendRequest{
		Endpoint: delivery.Endpoint,
		P256dh:   delivery.P256dh,
		Auth:     delivery.Auth,
		Body:     body,
		TTL:      ttl,
		Urgency:  urgency,
		Topic:    topic,
	})

	switch {
	case outcome.Gone:
		// The only signal a push service gives that a device is gone for good.
		// Delete first: if the finalize below fails, a stale row is cheaper
		// than a subscription that keeps consuming fan-out forever.
		if err := u.subscriptions.DeleteByEndpoint(ctx, delivery.UserID, delivery.Endpoint); err != nil {
			slog.ErrorContext(ctx, "push_subscription_delete_failed",
				"delivery_id", delivery.ID, "error", err)
		}
		u.finalize(ctx, delivery, outcome.StatusCode, "subscription gone")
		stats.Gone++
		stats.record(outcome.StatusCode, "gone")

	case outcome.Retryable:
		if delivery.Attempts >= maxAttempts {
			u.finalize(ctx, delivery, outcome.StatusCode, "retry budget exhausted")
			stats.Dead++
			stats.record(outcome.StatusCode, "dead")
			return
		}

		// A push service that asked for a delay knows something we do not;
		// overriding it with our own backoff is how a 429 becomes a longer one.
		wait := outcome.RetryAfter
		if wait <= 0 {
			wait = u.backoff(delivery.Attempts)
		}

		reason := "retryable"
		if sendErr != nil {
			reason = "transport failure"
		}
		if err := u.deliveries.Release(ctx, delivery.ID, wait, outcome.StatusCode, reason); err != nil {
			slog.ErrorContext(ctx, "push_delivery_release_failed",
				"delivery_id", delivery.ID, "error", err)
			return
		}
		stats.Released++
		stats.record(outcome.StatusCode, "released")

	case sendErr != nil:
		// Not classified as retryable and not a status we recognise: treat it
		// as terminal rather than looping on something we cannot characterise.
		u.finalize(ctx, delivery, outcome.StatusCode, "unclassified send failure")
		stats.Dead++
		stats.record(outcome.StatusCode, "dead")

	case outcome.StatusCode >= 200 && outcome.StatusCode < 300:
		if err := u.deliveries.MarkSent(ctx, delivery.ID, outcome.StatusCode); err != nil {
			slog.ErrorContext(ctx, "push_delivery_mark_sent_failed",
				"delivery_id", delivery.ID, "error", err)
			return
		}
		stats.Sent++
		stats.record(outcome.StatusCode, "sent")

	default:
		// 400 / 401 / 403 / 413 — our bug, not the device's. Retrying
		// reproduces it exactly. The 401/403 alert is what gets a human here;
		// the subscription is deliberately left alone, because a malformed
		// request of ours is no evidence about the user's device.
		u.finalize(ctx, delivery, outcome.StatusCode, "terminal rejection")
		stats.Dead++
		stats.record(outcome.StatusCode, "dead")
	}
}

// BacklogAge reports the queue's freshness for the gauge the stalled-delivery
// alert watches. Kept separate from DispatchBatch because it must be published
// on every pass, including passes that claim nothing — a gauge that stops being
// written keeps serving its last value, so a silent dispatcher would read
// healthy indefinitely.
func (u *Usecase) BacklogAge(ctx context.Context) (time.Duration, int64, int64, error) {
	return u.deliveries.BacklogAge(ctx)
}

func (s *Stats) record(statusCode int, result string) {
	s.Attempts = append(s.Attempts, Attempt{StatusCode: statusCode, Result: result})
}

func (u *Usecase) finalize(ctx context.Context, delivery domain.PushDelivery, status int, reason string) {
	if err := u.deliveries.MarkDead(ctx, delivery.ID, status, reason); err != nil {
		// The row keeps its lease and will be re-claimed, which is the safe
		// direction: a duplicate notification is survivable, a row stuck in
		// 'sending' forever is what makes the backlog-age alert lie.
		slog.ErrorContext(ctx, "push_delivery_finalize_failed",
			"delivery_id", delivery.ID, "reason", reason, "error", err)
	}
}
