// Package push_dispatch_port declares what the Web Push dispatcher needs from
// the outside world.
//
// SendOutcome is redeclared here rather than reusing the driver's SendResult so
// that the usecase depends on nothing below the port line. It carries the same
// three decisions because those are the only three the dispatcher can act on:
// delete the subscription, try again later, or stop.
package push_dispatch_port

import (
	"context"
	"time"

	"alt/domain"
)

// SendOutcome is what one delivery attempt tells the dispatcher to do next.
//
// Gone is separate from Retryable rather than folded into an error because a
// 404 or 410 obliges the caller to delete the subscription, and an obligation
// spelled as an error string is one a caller can miss by only checking err.
type SendOutcome struct {
	StatusCode int
	// Gone reports 404 or 410 — the device is unreachable for good, and its
	// subscription row must go. This is the only signal a push service gives
	// that an app was uninstalled or permission revoked.
	Gone bool
	// Retryable reports 429, 5xx or a transport failure.
	Retryable bool
	// RetryAfter is the push service's own requested delay, when it sent one.
	// It outranks any backoff the dispatcher would have computed.
	RetryAfter time.Duration
}

// DeliveryPort is the push_deliveries queue, reached over mTLS via
// alt-data-hub — cmd/notifier links no database driver.
type DeliveryPort interface {
	// ClaimBatch leases rows whose next_attempt_at has passed. The lease is
	// next_attempt_at itself, so a row abandoned by a crashed dispatcher is
	// re-claimed by the same query rather than by a separate sweeper.
	ClaimBatch(ctx context.Context, limit int) ([]domain.PushDelivery, error)
	MarkSent(ctx context.Context, deliveryID string, statusCode int) error
	// MarkDead ends the row's life. reason is kept short and free of the
	// endpoint, which is a capability URL.
	MarkDead(ctx context.Context, deliveryID string, statusCode int, reason string) error
	// Release returns a row to the queue with a new attempt time.
	Release(ctx context.Context, deliveryID string, retryAfter time.Duration, statusCode int, reason string) error
	// BacklogAge reports how old the oldest undelivered row is, how many are
	// waiting, and how many devices are registered to receive them.
	//
	// The dispatcher cannot answer this from what it claimed: a backlog that is
	// not being drained is precisely the case where this process claims
	// nothing, so deriving the age from a claimed batch would report zero for
	// the outage it is meant to reveal.
	BacklogAge(ctx context.Context) (oldestAge time.Duration, pending, subscriptions int64, err error)
}

// SubscriptionPort is the subscription registry, for the delete-on-gone path.
type SubscriptionPort interface {
	// Scoped by user as well as endpoint: the data plane refuses to delete a
	// row it cannot attribute, so a dispatcher bug cannot unsubscribe someone
	// else's device.
	DeleteByEndpoint(ctx context.Context, userID, endpoint string) error
}

// SendRequest is one fully-decided delivery attempt.
//
// The body arrives already rendered, and TTL / Urgency / Topic already chosen,
// because those are product decisions — how long a "your recap is ready" stays
// true, whether a digest may wait for Wi-Fi — and belong with the rest of the
// notification policy rather than in a transport adapter.
type SendRequest struct {
	// Endpoint is a capability URL. Never log it.
	Endpoint string
	P256dh   string
	Auth     string
	Body     []byte
	// TTL is mandatory in RFC 8030; omitting the header is a flat 400 from
	// every push service.
	TTL time.Duration
	// Urgency is the RFC 8030 vocabulary ("very-low" | "low" | "normal" |
	// "high"). Empty means normal, which is sent by omitting the header.
	Urgency string
	// Topic replaces an undelivered message with the same topic for this
	// subscription. It is neither encrypted nor authenticated and is never
	// forwarded to the user agent, so it must not carry an identifier.
	Topic string
}

// SenderPort talks to the push service.
type SenderPort interface {
	Send(ctx context.Context, req SendRequest) (SendOutcome, error)
}
