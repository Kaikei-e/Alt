// Package push_dispatch_gateway adapts the dispatcher's ports onto the two
// things it actually talks to: the data plane over mTLS, and the push services
// over HTTPS.
//
// It exists because the two sides disagree in small ways that should not reach
// the usecase — the data plane wants an absolute next-attempt time and a lease
// holder, the usecase reasons in "try again in this long"; the push driver has
// its own Urgency type, the usecase speaks the RFC's vocabulary. Translating
// here keeps the delivery policy testable without a network.
package push_dispatch_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/orchestrator/port/push_dispatch_port"
	"alt/shared/driver/webpush"
	"alt/shared/gateway/datahub_gateway"
)

// claimLease bounds how long a claimed row stays invisible to other
// dispatchers. It has to comfortably exceed one send — the driver's own client
// timeout is an order of magnitude smaller — because the lease is what makes a
// crash recoverable: the row's next_attempt_at is the lease, so an abandoned
// row simply becomes claimable again rather than needing a sweeper.
const claimLease = 60 * time.Second

// DeliveryGateway adapts push_deliveries.
type DeliveryGateway struct {
	inner *datahub_gateway.PushDeliveryGateway
	// lockedBy identifies this dispatcher instance in the claim, so a stuck
	// lease can be attributed to a container rather than guessed at.
	lockedBy string
	nowFn    func() time.Time
}

func NewDeliveryGateway(inner *datahub_gateway.PushDeliveryGateway, lockedBy string) *DeliveryGateway {
	if inner == nil {
		panic("push dispatch gateway: delivery gateway is nil — must be wired at the composition root")
	}
	return &DeliveryGateway{inner: inner, lockedBy: lockedBy, nowFn: time.Now}
}

func (g *DeliveryGateway) ClaimBatch(ctx context.Context, limit int) ([]domain.PushDelivery, error) {
	return g.inner.ClaimBatch(ctx, g.lockedBy, limit, claimLease)
}

func (g *DeliveryGateway) MarkSent(ctx context.Context, deliveryID string, statusCode int) error {
	return g.inner.MarkSent(ctx, deliveryID, statusCode)
}

func (g *DeliveryGateway) MarkDead(ctx context.Context, deliveryID string, statusCode int, reason string) error {
	return g.inner.MarkDead(ctx, deliveryID, statusCode, reason)
}

func (g *DeliveryGateway) Release(ctx context.Context, deliveryID string, retryAfter time.Duration, statusCode int, reason string) error {
	// The reason carries the status because the data plane's Release takes only
	// a message; losing the code would make "why is this row still retrying"
	// unanswerable from the row alone.
	message := fmt.Sprintf("%s (status %d)", reason, statusCode)
	return g.inner.Release(ctx, deliveryID, g.nowFn().Add(retryAfter), message)
}

func (g *DeliveryGateway) BacklogAge(ctx context.Context) (time.Duration, int64, error) {
	return g.inner.BacklogAge(ctx)
}

// SubscriptionGateway adapts the subscription registry for the delete-on-gone
// path. It is the only mutation the dispatcher performs on a user's data.
type SubscriptionGateway struct {
	inner *datahub_gateway.PushSubscriptionGateway
}

func NewSubscriptionGateway(inner *datahub_gateway.PushSubscriptionGateway) *SubscriptionGateway {
	if inner == nil {
		panic("push dispatch gateway: subscription gateway is nil — the 410 branch would keep dead devices in the fan-out")
	}
	return &SubscriptionGateway{inner: inner}
}

func (g *SubscriptionGateway) DeleteByEndpoint(ctx context.Context, userID, endpoint string) error {
	_, err := g.inner.Delete(ctx, userID, endpoint)
	return err
}

// Sender adapts the RFC 8030/8291/8292 driver.
type Sender struct {
	client *webpush.Client
}

func NewSender(client *webpush.Client) *Sender {
	if client == nil {
		panic("push dispatch gateway: web push client is nil — the queue would drain into nothing")
	}
	return &Sender{client: client}
}

func urgency(v string) webpush.Urgency {
	switch v {
	case "very-low":
		return webpush.UrgencyVeryLow
	case "low":
		return webpush.UrgencyLow
	case "high":
		return webpush.UrgencyHigh
	default:
		// Normal is the RFC default and is expressed by omitting the header,
		// which the driver does for this value.
		return webpush.UrgencyNormal
	}
}

func (s *Sender) Send(ctx context.Context, req push_dispatch_port.SendRequest) (push_dispatch_port.SendOutcome, error) {
	result, err := s.client.Send(ctx,
		webpush.Subscription{
			Endpoint: req.Endpoint,
			Keys: webpush.SubscriptionKeys{
				P256dh: req.P256dh,
				Auth:   req.Auth,
			},
		},
		webpush.Message{
			Payload: req.Body,
			TTL:     req.TTL,
			Urgency: urgency(req.Urgency),
			Topic:   req.Topic,
		},
	)

	// The result is returned even alongside an error, so the Gone and Retryable
	// decisions survive a transport failure rather than being lost to `err`.
	return push_dispatch_port.SendOutcome{
		StatusCode: result.StatusCode,
		Gone:       result.Gone,
		Retryable:  result.Retryable,
		RetryAfter: result.RetryAfter,
	}, err
}
